package main

import (
	"flag"
	"fmt"
	"net/http"
	"os"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	log "github.com/sirupsen/logrus"
	"gitlab.com/gitlab-org/gitaly/internal/config"
	"gitlab.com/gitlab-org/gitaly/internal/git"
	"gitlab.com/gitlab-org/gitaly/internal/linguist"
	"gitlab.com/gitlab-org/gitaly/internal/server"
	"gitlab.com/gitlab-org/gitaly/internal/tempdir"
	"gitlab.com/gitlab-org/gitaly/internal/version"
	"gitlab.com/gitlab-org/labkit/tracing"
)

var (
	flagVersion = flag.Bool("version", false, "Print version and exit")
)

func loadConfig(configPath string) error {
	cfgFile, err := os.Open(configPath)
	if err != nil {
		return err
	}
	defer cfgFile.Close()

	if err = config.Load(cfgFile); err != nil {
		return err
	}

	if err := config.Validate(); err != nil {
		return err
	}

	if err := linguist.LoadColors(); err != nil {
		return fmt.Errorf("load linguist colors: %v", err)
	}

	return nil
}

// registerServerVersionPromGauge registers a label with the current server version
// making it easy to see what versions of Gitaly are running across a cluster
func registerServerVersionPromGauge() {
	gitVersion, err := git.Version()
	if err != nil {
		fmt.Printf("git version: %v\n", err)
		os.Exit(1)
	}
	gitlabBuildInfoGauge := prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "gitlab_build_info",
		Help: "Current build info for this GitLab Service",
		ConstLabels: prometheus.Labels{
			"version":     version.GetVersion(),
			"built":       version.GetBuildTime(),
			"git_version": gitVersion,
		},
	})

	prometheus.MustRegister(gitlabBuildInfoGauge)
	gitlabBuildInfoGauge.Set(1)
}

func flagUsage() {
	fmt.Println(version.GetVersionString())
	fmt.Printf("Usage: %v [OPTIONS] configfile\n", os.Args[0])
	flag.PrintDefaults()
}

func main() {
	flag.Usage = flagUsage
	flag.Parse()

	// gitaly-wrapper is supposed to set config.EnvUpgradesEnabled in order to enable graceful upgrades
	_, isWrapped := os.LookupEnv(config.EnvUpgradesEnabled)
	b, err := newBootstrap(os.Getenv(config.EnvPidFile), isWrapped)
	if err != nil {
		log.WithError(err).Fatal("init bootstrap")
	}
	defer b.Stop()

	// If invoked with -version
	if *flagVersion {
		fmt.Println(version.GetVersionString())
		os.Exit(0)
	}

	if flag.NArg() != 1 || flag.Arg(0) == "" {
		flag.Usage()
		os.Exit(2)
	}

	log.WithField("version", version.GetVersionString()).Info("Starting Gitaly")
	registerServerVersionPromGauge()

	configPath := flag.Arg(0)
	if err := loadConfig(configPath); err != nil {
		log.WithError(err).WithField("config_path", configPath).Fatal("load config")
	}

	config.ConfigureLogging()
	config.ConfigureSentry(version.GetVersion())
	config.ConfigurePrometheus()
	config.ConfigureConcurrencyLimits()
	tracing.Initialize(tracing.WithServiceName("gitaly"))

	tempdir.StartCleaning()

	if err = b.listen(); err != nil {
		log.WithError(err).Fatal("bootstrap failed")
	}

	if config.Config.PrometheusListenAddr != "" {
		l, err := b.prometheusListener()
		if err != nil {
			log.WithError(err).Fatal("configure prometheus listener")
		}

		promMux := http.NewServeMux()
		promMux.Handle("/metrics", promhttp.Handler())

		server.AddPprofHandlers(promMux)

		go func() {
			err = http.Serve(l, promMux)
			if err != nil {
				log.WithError(err).Fatal("Unable to serve prometheus")
			}
		}()
	}

	b.run()

	log.Fatal("shutting down")
}
