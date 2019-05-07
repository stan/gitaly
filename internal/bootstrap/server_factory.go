package bootstrap

import (
	"net"
	"sync"

	log "github.com/sirupsen/logrus"
	"gitlab.com/gitlab-org/gitaly/internal/rubyserver"
	"gitlab.com/gitlab-org/gitaly/internal/server"
	"google.golang.org/grpc"
)

type serverFactory struct {
	ruby             *rubyserver.Server
	secure, insecure *grpc.Server
}

// GracefulStoppableServer allows to serve contents on a net.Listener, Stop serving and performing a GracefulStop
type GracefulStoppableServer interface {
	GracefulStop()
	Stop()
	Serve(l net.Listener, secure bool) error
}

// NewServerFactory initializes a rubyserver and then lazily initializes both secure and insecure grpc.Server
func NewServerFactory() (GracefulStoppableServer, error) {
	ruby, err := rubyserver.Start()
	if err != nil {
		log.Error("start ruby server")

		return nil, err
	}

	return &serverFactory{ruby: ruby}, nil
}

func (s *serverFactory) Stop() {
	for _, srv := range s.all() {
		srv.Stop()
	}

	s.ruby.Stop()
}

func (s *serverFactory) GracefulStop() {
	wg := sync.WaitGroup{}

	for _, srv := range s.all() {
		wg.Add(1)

		go func(s *grpc.Server) {
			s.GracefulStop()
			wg.Done()
		}(srv)
	}

	wg.Wait()
}

func (s *serverFactory) Serve(l net.Listener, secure bool) error {
	srv := s.get(secure)

	return srv.Serve(l)
}

func (s *serverFactory) get(secure bool) *grpc.Server {
	if secure {
		if s.secure == nil {
			s.secure = server.NewSecure(s.ruby)
		}

		return s.secure
	}

	if s.insecure == nil {
		s.insecure = server.NewInsecure(s.ruby)
	}

	return s.insecure
}

func (s *serverFactory) all() []*grpc.Server {
	servers := make([]*grpc.Server, 0, 2)
	if s.secure != nil {
		servers = append(servers, s.secure)
	}

	if s.insecure != nil {
		servers = append(servers, s.insecure)
	}

	return servers
}
