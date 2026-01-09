package crds

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParse(t *testing.T) {
	crds, err := Parse()
	assert.NoError(t, err)
	assert.Len(t, crds, 13)
}
