package telemetry

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"time"

	"github.com/jzelinskie/cobrautil"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/rs/zerolog/log"
	"golang.org/x/sync/errgroup"

	"github.com/authzed/spicedb/internal/datastore"
	"github.com/authzed/spicedb/internal/middleware/usagemetrics"
)

var (
	Registry                  = prometheus.NewRegistry()
	dynamicDispatchHistLabels = []string{"method", "cached"}
)

// RegisterTelemetryCollector registers a collector for the various pieces of
// data required by SpiceDB telemetry.
func RegisterTelemetryCollector(datastoreEngine string, ds datastore.Datastore) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	nodeID, err := os.Hostname()
	if err != nil {
		return fmt.Errorf("unable to get hostname: %w", err)
	}

	dbStats, err := ds.Statistics(ctx)
	if err != nil {
		return fmt.Errorf("unable to query DB stats: %w", err)
	}

	clusterID := dbStats.UniqueID

	if err := Registry.Register(&collector{
		ds: ds,
		infoDesc: prometheus.NewDesc(
			prometheus.BuildFQName("spicedb", "telemetry", "info"),
			"Information about the SpiceDB environment.",
			nil,
			prometheus.Labels{
				"cluster_id": clusterID,
				"node_id":    nodeID,
				"version":    cobrautil.Version,
				"os":         runtime.GOOS,
				"arch":       runtime.GOARCH,
				"vcpu":       fmt.Sprintf("%d", runtime.NumCPU()),
				"ds_engine":  datastoreEngine,
			},
		),
		objectDefsDesc: prometheus.NewDesc(
			prometheus.BuildFQName("spicedb", "telemetry", "object_definitions_total"),
			"Count of the number of objects defined by the schema.",
			nil,
			prometheus.Labels{
				"cluster_id": clusterID,
				"node_id":    nodeID,
			},
		),
		relationshipsDesc: prometheus.NewDesc(
			prometheus.BuildFQName("spicedb", "telemetry", "relationships_estimate_total"),
			"Count of the estimated number of stored relationships.",
			nil,
			prometheus.Labels{
				"cluster_id": clusterID,
				"node_id":    nodeID,
			},
		),
		dispatchedDesc: prometheus.NewDesc(
			prometheus.BuildFQName("spicedb", "telemetry", "dispatches"),
			"Histogram of cluster dispatches performed by the instance.",
			dynamicDispatchHistLabels,
			prometheus.Labels{
				"cluster_id": clusterID,
				"node_id":    nodeID,
			},
		),
	}); err != nil {
		return fmt.Errorf("unable to register telemetry collector: %w", err)
	}

	return nil
}

type collector struct {
	ds                datastore.Datastore
	infoDesc          *prometheus.Desc
	objectDefsDesc    *prometheus.Desc
	relationshipsDesc *prometheus.Desc
	dispatchedDesc    *prometheus.Desc
}

func (c *collector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.infoDesc
	ch <- c.objectDefsDesc
	ch <- c.relationshipsDesc
	ch <- c.dispatchedDesc
}

func (c *collector) Collect(ch chan<- prometheus.Metric) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	dsStats, err := c.ds.Statistics(ctx)
	if err != nil {
		log.Warn().Err(err).Msg("unable to collect datastore statistics")
	}

	ch <- prometheus.MustNewConstMetric(c.infoDesc, prometheus.GaugeValue, 1)
	ch <- prometheus.MustNewConstMetric(c.objectDefsDesc, prometheus.GaugeValue, float64(len(dsStats.ObjectTypeStatistics)))
	ch <- prometheus.MustNewConstMetric(c.relationshipsDesc, prometheus.GaugeValue, float64(dsStats.EstimatedRelationshipCount))

	dispatchedCountMetrics := make(chan prometheus.Metric)
	g := errgroup.Group{}
	g.Go(func() error {
		for metric := range dispatchedCountMetrics {
			var m dto.Metric
			if err := metric.Write(&m); err != nil {
				return fmt.Errorf("error writing metric: %w", err)
			}

			buckets := make(map[float64]uint64, len(m.Histogram.Bucket))
			for _, bucket := range m.Histogram.Bucket {
				buckets[*bucket.UpperBound] = *bucket.CumulativeCount
			}

			dynamicLabels := make([]string, len(dynamicDispatchHistLabels))
			for i, labelName := range dynamicDispatchHistLabels {
				for _, labelVal := range m.Label {
					if *labelVal.Name == labelName {
						dynamicLabels[i] = *labelVal.Value
					}
				}
			}
			ch <- prometheus.MustNewConstHistogram(
				c.dispatchedDesc,
				*m.Histogram.SampleCount,
				*m.Histogram.SampleSum,
				buckets,
				dynamicLabels...,
			)
		}
		return nil
	})

	usagemetrics.DispatchedCountHistogram.Collect(dispatchedCountMetrics)
	close(dispatchedCountMetrics)

	if err := g.Wait(); err != nil {
		log.Error().Err(err).Msg("error collecting metrics")
	}
}
