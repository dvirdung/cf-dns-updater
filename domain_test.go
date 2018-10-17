package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetBaseDomain(t *testing.T) {

	d := getBaseDomain("asdf.test.jonaz.net")
	assert.Equal(t, "jonaz.net", d)
}
