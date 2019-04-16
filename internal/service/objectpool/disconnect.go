package objectpool

import (
	"context"

	"gitlab.com/gitlab-org/gitaly-proto/go/gitalypb"
	"gitlab.com/gitlab-org/gitaly/internal/helper"
)

func (s *server) DisconnectGitAlternates(context.Context, *gitalypb.DisconnectGitAlternatesRequest) (*gitalypb.DisconnectGitAlternatesResponse, error) {
	return nil, helper.Unimplemented
}
