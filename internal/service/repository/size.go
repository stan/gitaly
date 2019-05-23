package repository

import (
	"bytes"
	"context"
	"fmt"
	"io/ioutil"
	"os/exec"
	"strconv"

	grpc_logrus "github.com/grpc-ecosystem/go-grpc-middleware/logging/logrus"
	"gitlab.com/gitlab-org/gitaly-proto/go/gitalypb"
	"gitlab.com/gitlab-org/gitaly/internal/command"
	"gitlab.com/gitlab-org/gitaly/internal/helper"
)

func (s *server) RepositorySize(ctx context.Context, in *gitalypb.RepositorySizeRequest) (*gitalypb.RepositorySizeResponse, error) {
	path, err := helper.GetPath(in.Repository)
	if err != nil {
		return nil, err
	}

	return &gitalypb.RepositorySizeResponse{Size: getPathSize(ctx, path)}, nil
}

func (s *server) GetObjectDirectorySize (ctx context.Context, in *gitalypb.GetObjectDirectorySizeRequest) (*gitalypb.GetObjectDirectorySizeResponse, error) {
	path, err := helper.GetObjectDirectoryPath(in.Repository)
	if err != nil {
		return nil, err
	}

	return &gitalypb.GetObjectDirectorySizeResponse{Size: getPathSize(ctx, path)}, nil
}

func getPathSize(ctx context.Context, path string) (int64) {
	cmd, err := command.New(ctx, exec.Command("du", "-sk", path), nil, nil, nil)
	if err != nil {
		grpc_logrus.Extract(ctx).WithError(err).Warn("ignoring du command error")
		return 0
	}

	sizeLine, err := ioutil.ReadAll(cmd)
	if err != nil {
		grpc_logrus.Extract(ctx).WithError(err).Warn("ignoring command read error")
		return 0
	}

	if err := cmd.Wait(); err != nil {
		grpc_logrus.Extract(ctx).WithError(err).Warn("ignoring du wait error")
		return 0
	}

	sizeParts := bytes.Split(sizeLine, []byte("\t"))
	if len(sizeParts) != 2 {
		grpc_logrus.Extract(ctx).Warn(fmt.Sprintf("ignoring du malformed output: %q", sizeLine))
		return 0
	}

	size, err := strconv.ParseInt(string(sizeParts[0]), 10, 0)
	if err != nil {
		grpc_logrus.Extract(ctx).WithError(err).Warn("ignoring parsing size error")
		return 0
	}

	return size
}
