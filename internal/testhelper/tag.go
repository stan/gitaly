package testhelper

import (
	"bytes"
	"fmt"
	"testing"

	"gitlab.com/gitlab-org/gitaly/internal/helper/text"
)

// CreateTagOpts holds extra options for CreateTag.
type CreateTagOpts struct {
	Message string
}

// CreateTag creates a new tag.
func CreateTag(t *testing.T, repoPath, tagName, targetID string, opts *CreateTagOpts) string {
	var message string

	if opts != nil {
		if opts.Message != "" {
			message = opts.Message
		}
	}

	committerName := "Scrooge McDuck"
	committerEmail := "scrooge@mcduck.com"

	// message can be very large, passing it directly in args would blow things up!
	stdin := bytes.NewBufferString(message)

	args := []string{"-C", repoPath,
		"-c", fmt.Sprintf("user.name=%s", committerName),
		"-c", fmt.Sprintf("user.email=%s", committerEmail),
		"tag",
	}
	if message != "" {
		args = append(args, "-F", "-")
	}
	args = append(args, tagName, targetID)

	MustRunCommand(t, stdin, "git", args...)

	tagID := MustRunCommand(t, nil, "git", "-C", repoPath, "show-ref", "-s", tagName)
	return text.ChompBytes(tagID)
}
