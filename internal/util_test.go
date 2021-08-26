package internal

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestContainsString(t *testing.T) {
	assert.True(t, ContainsString([]string{"a", "b", "c"}, "a"))
	assert.True(t, ContainsString([]string{"a", "b", "c"}, "b"))
	assert.True(t, ContainsString([]string{"a", "b", "c"}, "c"))
	assert.False(t, ContainsString([]string{"a", "b", "c"}, ""))
	assert.False(t, ContainsString([]string{"a", "b", "c"}, "aa"))
	assert.False(t, ContainsString([]string{"a", "b", "c"}, "ab"))
	assert.False(t, ContainsString([]string{"a", "b", "c"}, "abc"))
	assert.False(t, ContainsString([]string{"a", "b", "c"}, "d"))
	assert.False(t, ContainsString(nil, "a"))
	assert.False(t, ContainsString([]string{}, "a"))
	assert.False(t, ContainsString([]string{}, ""))
}

func TestRemoveString(t *testing.T) {
	assert.Equal(t, []string{"b", "c"}, RemoveString([]string{"a", "b", "c"}, "a"))
	assert.Equal(t, []string{"a", "c"}, RemoveString([]string{"a", "b", "c"}, "b"))
	assert.Equal(t, []string{"a", "b"}, RemoveString([]string{"a", "b", "c"}, "c"))
	assert.Equal(t, []string{"a", "b", "c"}, RemoveString([]string{"a", "b", "c"}, ""))
	assert.Equal(t, []string{"a", "b", "c"}, RemoveString([]string{"a", "b", "c"}, "d"))
	assert.Equal(t, []string{}, RemoveString([]string{}, "a"))
	assert.Equal(t, []string{}, RemoveString([]string{}, ""))
	assert.Equal(t, []string(nil), RemoveString(nil, ""))
	assert.Equal(t, []string(nil), RemoveString(nil, "a"))
}
