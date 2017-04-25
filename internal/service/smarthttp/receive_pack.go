package smarthttp

import (
	"fmt"
	"log"
	"os/exec"

	"gitlab.com/gitlab-org/gitaly/internal/helper"

	pb "gitlab.com/gitlab-org/gitaly-proto/go"
	pbhelper "gitlab.com/gitlab-org/gitaly-proto/go/helper"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
)

func (s *server) PostReceivePack(stream pb.SmartHTTP_PostReceivePackServer) error {
	req, err := stream.Recv() // First request contains only Repository and GlId
	if err != nil {
		return err
	}
	if err := validateReceivePackRequest(req); err != nil {
		return err
	}

	stdin := pbhelper.NewReceiveReader(func() ([]byte, error) {
		resp, err := stream.Recv()
		return resp.GetData(), err
	})
	stdout := pbhelper.NewSendWriter(func(p []byte) error {
		return stream.Send(&pb.PostReceivePackResponse{Data: p})
	})
	glIDEnv := fmt.Sprintf("GL_ID=%s", req.GlId)
	repoPath, err := helper.GetRepoPath(req.Repository)
	if err != nil {
		return err
	}

	log.Printf("PostReceivePack: RepoPath=%q GlID=%q", repoPath, req.GlId)

	osCommand := exec.Command("git", "receive-pack", "--stateless-rpc", repoPath)
	cmd, err := helper.NewCommand(osCommand, stdin, stdout, glIDEnv)

	if err != nil {
		return grpc.Errorf(codes.Unavailable, "PostReceivePack: cmd: %v", err)
	}
	defer cmd.Kill()

	if err := cmd.Wait(); err != nil {
		return grpc.Errorf(codes.Unavailable, "PostReceivePack: cmd wait for %v: %v", cmd.Args, err)
	}

	return nil
}

func validateReceivePackRequest(req *pb.PostReceivePackRequest) error {
	if req.GlId == "" {
		return grpc.Errorf(codes.InvalidArgument, "PostReceivePack: empty GlId")
	}
	if req.Data != nil {
		return grpc.Errorf(codes.InvalidArgument, "PostReceivePack: non-empty Data")
	}

	return nil
}
