// Copyright 2020 Burak Sezer
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"net/http"
	"os"
	"time"

	"github.com/buraksezer/olric/client"
	"github.com/buraksezer/olric/serializer"
	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/prometheus/common/promlog"
	"github.com/prometheus/common/promlog/flag"
	"github.com/prometheus/common/version"
	"gopkg.in/alecthomas/kingpin.v2"
)

type Exporter struct {
	address string
	timeout time.Duration
	logger  log.Logger

	up *prometheus.Desc
}

// NewExporter returns an initialized exporter.
func NewExporter(server string, timeout time.Duration, logger log.Logger) *Exporter {
	return &Exporter{
		address: server,
		timeout: timeout,
		logger:  logger,
		up: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "", "up"),
			"Could the Olric server be reached.",
			nil,
			nil,
		),
	}
}

// Collect fetches the statistics from the configured Olric server, and
// delivers them as Prometheus metrics. It implements prometheus.Collector.
func (e *Exporter) Collect(ch chan<- prometheus.Metric) {
	cc := &client.Config{
		Addrs:       []string{e.address},
		MaxConn:     10,
		Serializer:  serializer.NewMsgpackSerializer(),
		DialTimeout: e.timeout,
	}
	c, err := client.New(cc)
	if err != nil {
		ch <- prometheus.MustNewConstMetric(e.up, prometheus.GaugeValue, 0)
		level.Error(e.logger).Log("msg", "Failed to connect to Olric", "err", err)
		return
	}

	up := float64(1)
	_, err = c.Stats(e.address)
	if err != nil {
		level.Error(e.logger).Log("msg", "Failed to collect stats from memcached", "err", err)
		up = 0
	}

	ch <- prometheus.MustNewConstMetric(e.up, prometheus.GaugeValue, up)
}

// Describe describes all the metrics exported by the olric exporter. It
// implements prometheus.Collector.
func (e *Exporter) Describe(ch chan<- *prometheus.Desc) {
}

const namespace = "olric"

func main() {
	var (
		address       = kingpin.Flag("olric.address", "Olric server address.").Default("localhost:3320").String()
		timeout       = kingpin.Flag("olric.timeout", "olric connect timeout.").Default("1s").Duration()
		listenAddress = kingpin.Flag("web.listen-address", "Address to listen on for web interface and telemetry.").Default(":9150").String()
		metricsPath   = kingpin.Flag("web.telemetry-path", "Path under which to expose metrics.").Default("/metrics").String()
	)
	promlogConfig := &promlog.Config{}
	flag.AddFlags(kingpin.CommandLine, promlogConfig)
	kingpin.HelpFlag.Short('h')
	kingpin.Parse()
	logger := promlog.New(promlogConfig)

	level.Info(logger).Log("msg", "Starting olric_exporter", "version", version.Info())
	level.Info(logger).Log("msg", "Build context", "context", version.BuildContext())

	prometheus.MustRegister(NewExporter(*address, *timeout, logger))
	http.Handle(*metricsPath, promhttp.Handler())
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<html>
             <head><title>Olric Exporter</title></head>
             <body>
             <h1>Olric Exporter</h1>
             <p><a href='` + *metricsPath + `'>Metrics</a></p>
             </body>
             </html>`))
	})

	level.Info(logger).Log("msg", "Listening on address", "address", *listenAddress)
	if err := http.ListenAndServe(*listenAddress, nil); err != nil {
		level.Error(logger).Log("msg", "Error running HTTP server", "err", err)
		os.Exit(1)
	}
}
