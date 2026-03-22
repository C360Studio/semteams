package component

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestStoreReadPort_ResourceID(t *testing.T) {
	port := StoreReadPort{Bucket: "MESSAGES"}
	assert.Equal(t, "store-read:MESSAGES", port.ResourceID())
}

func TestStoreReadPort_IsExclusive(t *testing.T) {
	port := StoreReadPort{Bucket: "MESSAGES"}
	assert.False(t, port.IsExclusive(), "multiple readers should be allowed")
}

func TestStoreReadPort_Type(t *testing.T) {
	port := StoreReadPort{Bucket: "MESSAGES"}
	assert.Equal(t, "store-read", port.Type())
}

func TestBuildPortFromDefinition_StoreRead(t *testing.T) {
	def := PortDefinition{
		Name:   "content_store",
		Type:   "store-read",
		Bucket: "MESSAGES",
	}

	port := BuildPortFromDefinition(def, DirectionInput)

	assert.Equal(t, "content_store", port.Name)
	storePort, ok := port.Config.(StoreReadPort)
	assert.True(t, ok, "config should be StoreReadPort")
	assert.Equal(t, "MESSAGES", storePort.Bucket)
}

func TestBuildPortFromDefinition_StoreRead_FallbackToSubject(t *testing.T) {
	def := PortDefinition{
		Name:    "content_store",
		Type:    "store-read",
		Subject: "ARCHIVE",
	}

	port := BuildPortFromDefinition(def, DirectionInput)

	storePort, ok := port.Config.(StoreReadPort)
	assert.True(t, ok)
	assert.Equal(t, "ARCHIVE", storePort.Bucket, "should fall back to Subject when Bucket is empty")
}
