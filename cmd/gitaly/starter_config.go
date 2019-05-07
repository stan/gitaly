package main

import (
	"github.com/sirupsen/logrus"
	"gitlab.com/gitlab-org/gitaly/internal/bootstrap"
)

const (
	tcp  string = "tcp"
	tls  string = "tls"
	unix string = "unix"
)

type starterConfig struct {
	name, addr string
}

func (s *starterConfig) isSecure() bool {
	return s.name == tls
}

func (s *starterConfig) family() string {
	if s.isSecure() {
		return tcp
	}

	return s.name
}

func gitalyStarter(cfg starterConfig, servers bootstrap.GracefulStoppableServer) bootstrap.Starter {
	return func(listen bootstrap.ListenFunc, errCh chan<- error) error {
		l, err := listen(cfg.family(), cfg.addr)
		if err != nil {
			return err
		}

		logrus.WithField("address", cfg.addr).Infof("listening at %s address", cfg.name)

		go func() {
			errCh <- servers.Serve(l, cfg.isSecure())
		}()

		return nil
	}
}
