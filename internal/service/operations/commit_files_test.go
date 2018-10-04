	"fmt"
	"gitlab.com/gitlab-org/gitaly-proto/go/gitalypb"
	user = &gitalypb.User{
		desc            string
		repo            *gitalypb.Repository
		repoPath        string
		branchName      string
		repoCreated     bool
		branchCreated   bool
		executeFilemode bool
		{
			desc:            "create executable file",
			repo:            testRepo,
			repoPath:        testRepoPath,
			branchName:      "feature-executable",
			repoCreated:     false,
			branchCreated:   true,
			executeFilemode: true,
		},
			actionsRequest4 := chmodFileHeaderRequest(filePath, tc.executeFilemode)
			require.NoError(t, stream.Send(actionsRequest4))

			commitInfo := testhelper.MustRunCommand(t, nil, "git", "-C", tc.repoPath, "show", headCommit.GetId())
			expectedFilemode := "100644"
			if tc.executeFilemode {
				expectedFilemode = "100755"
			}
			require.Contains(t, string(commitInfo), fmt.Sprint("new file mode ", expectedFilemode))
		requests   []*gitalypb.UserCommitFilesRequest
			requests: []*gitalypb.UserCommitFilesRequest{
		{
			desc: "file doesn't exists",
			requests: []*gitalypb.UserCommitFilesRequest{
				headerRequest(testRepo, user, "feature", commitFilesMessage, nil, nil),
				chmodFileHeaderRequest("documents/story.txt", true),
			},
			indexError: "A file with this name doesn't exist",
		},
			requests: []*gitalypb.UserCommitFilesRequest{
				actionRequest(&gitalypb.UserCommitFilesAction{
					UserCommitFilesActionPayload: &gitalypb.UserCommitFilesAction_Header{
						Header: &gitalypb.UserCommitFilesActionHeader{
							Action:        gitalypb.UserCommitFilesActionHeader_CREATE_DIR,
		req  *gitalypb.UserCommitFilesRequest
func headerRequest(repo *gitalypb.Repository, user *gitalypb.User, branchName string, commitMessage, authorName, authorEmail []byte) *gitalypb.UserCommitFilesRequest {
	return &gitalypb.UserCommitFilesRequest{
		UserCommitFilesRequestPayload: &gitalypb.UserCommitFilesRequest_Header{
			Header: &gitalypb.UserCommitFilesRequestHeader{
func createFileHeaderRequest(filePath string) *gitalypb.UserCommitFilesRequest {
	return actionRequest(&gitalypb.UserCommitFilesAction{
		UserCommitFilesActionPayload: &gitalypb.UserCommitFilesAction_Header{
			Header: &gitalypb.UserCommitFilesActionHeader{
				Action:        gitalypb.UserCommitFilesActionHeader_CREATE,
func chmodFileHeaderRequest(filePath string, executeFilemode bool) *gitalypb.UserCommitFilesRequest {
	return actionRequest(&gitalypb.UserCommitFilesAction{
		UserCommitFilesActionPayload: &gitalypb.UserCommitFilesAction_Header{
			Header: &gitalypb.UserCommitFilesActionHeader{
				Action:          gitalypb.UserCommitFilesActionHeader_CHMOD,
				FilePath:        []byte(filePath),
				ExecuteFilemode: executeFilemode,
			},
		},
	})
}

func actionContentRequest(content string) *gitalypb.UserCommitFilesRequest {
	return actionRequest(&gitalypb.UserCommitFilesAction{
		UserCommitFilesActionPayload: &gitalypb.UserCommitFilesAction_Content{
func actionRequest(action *gitalypb.UserCommitFilesAction) *gitalypb.UserCommitFilesRequest {
	return &gitalypb.UserCommitFilesRequest{
		UserCommitFilesRequestPayload: &gitalypb.UserCommitFilesRequest_Action{