package fstype

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFileSystem(t *testing.T) {
	// Testing environments aren't stable, so this test is just to check if
	// compilation is successful and the function can be executed
	assert.NotEmpty(t, FileSystem("."))
}
