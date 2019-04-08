package main

import (
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	log "github.com/sirupsen/logrus"
	"gitlab.com/gitlab-org/gitaly/internal/bootstrap"
	"gitlab.com/gitlab-org/gitaly/internal/config"
	"gitlab.com/gitlab-org/gitaly/internal/connectioncounter"
	"gitlab.com/gitlab-org/gitaly/internal/git"
	"gitlab.com/gitlab-org/gitaly/internal/linguist"
	"gitlab.com/gitlab-org/gitaly/internal/rubyserver"
	"gitlab.com/gitlab-org/gitaly/internal/server"
	"gitlab.com/gitlab-org/gitaly/internal/tempdir"
	"gitlab.com/gitlab-org/gitaly/internal/version"
	"gitlab.com/gitlab-org/labkit/tracing"
	"google.golang.org/grpc"
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
	b, err := bootstrap.New(os.Getenv(config.EnvPidFile), isWrapped)
	if err != nil {
		log.WithError(err).Fatal("init bootstrap")
	}

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

	ruby, err := rubyserver.Start()
	if err != nil {
		log.WithError(err).Fatal("start ruby server")
	}
	defer ruby.Stop()

	insecureServer := server.NewInsecure(ruby)
	secureServer := server.NewSecure(ruby)

	for _, s := range []*grpc.Server{insecureServer, secureServer} {
		go func(s *grpc.Server) {
			<-b.GracefulStop
			s.GracefulStop()
		}(s)
	}

	if socketPath := config.Config.SocketPath; socketPath != "" {
		b.RegisterStarter(func(listen bootstrap.ListenFunc, errCh chan<- error) error {
			l, err := createUnixListener(listen, socketPath, b.IsFirstBoot())
			if err != nil {
				return err
			}

			log.WithField("address", socketPath).Info("listening on unix socket")
			go func() {
				errCh <- insecureServer.Serve(connectioncounter.New("unix", l))
			}()

			return nil
		})
	}

	type tcpConfig struct {
		name, addr string
		s          *grpc.Server
	}

	for _, cfg := range []tcpConfig{
		{name: "tcp", addr: config.Config.ListenAddr, s: insecureServer},
		{name: "tls", addr: config.Config.TLSListenAddr, s: secureServer},
	} {
		if cfg.addr == "" {
			continue
		}

		// be careful with closure over cfg inside for loop
		func(cfg tcpConfig) {
			b.RegisterStarter(func(listen bootstrap.ListenFunc, errCh chan<- error) error {
				l, err := listen("tcp", cfg.addr)
				if err != nil {
					return err
				}

				log.WithField("address", cfg.addr).Infof("listening at %s address", cfg.name)
				go func() {
					errCh <- cfg.s.Serve(connectioncounter.New(cfg.name, l))
				}()

				return nil
			})
		}(cfg)
	}

	if addr := config.Config.PrometheusListenAddr; addr != "" {
		b.RegisterExtraStarter(func(listen bootstrap.ListenFunc) error {
			l, err := listen("tcp", addr)
			if err != nil {
				return err
			}

			log.WithField("address", addr).Info("starting prometheus listener")

			promMux := http.NewServeMux()
			promMux.Handle("/metrics", promhttp.Handler())

			server.AddPprofHandlers(promMux)

			go func() {
				if err := http.Serve(l, promMux); err != nil {
					log.WithError(err).Fatal("Unable to serve prometheus")
				}
			}()

			return nil
		})
	}

	if err := b.Start(); err != nil {
		log.WithError(err).Fatal("unable to start listeners")
	}

	log.WithError(b.Wait()).Error("shutting down")
}

func createUnixListener(listen bootstrap.ListenFunc, socketPath string, removeOld bool) (net.Listener, error) {
	if removeOld {
		if err := os.Remove(socketPath); err != nil && !os.IsNotExist(err) {
			return nil, err
		}
	}

	return listen("unix", socketPath)
}
