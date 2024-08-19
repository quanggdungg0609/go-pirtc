package rtp_broker

import (
	"testing"
	"time"

	"github.com/pion/rtp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewBroker(t *testing.T) {
	broker, err := NewBroker("localhost", 12345)
	require.NoError(t, err)
	assert.NotNil(t, broker)
	assert.Equal(t, 12345, broker.port)
	assert.Equal(t, "localhost", broker.address)
	assert.NotNil(t, broker.listerner)
	assert.NotNil(t, broker.stopChan)
	assert.Empty(t, broker.subcriptions)
}

func TestStart(t *testing.T) {
	broker, err := NewBroker("localhost", 12345)
	require.NoError(t, err)
	err = broker.Start()
	require.NoError(t, err)
	assert.NotNil(t, broker.cmd)
	assert.NotNil(t, broker.ctx)
	assert.NotNil(t, broker.cancel)
}

func TestSubUnsub(t *testing.T) {
	broker, err := NewBroker("localhost", 12345)
	require.NoError(t, err)
	broker.Start()

	subID := "test-sub"
	packetChan := broker.Sub(subID)
	assert.NotNil(t, packetChan)
	assert.Contains(t, broker.subcriptions, subID)

	broker.UnSub(subID)
	assert.NotContains(t, broker.subcriptions, subID)
}

func TestStop(t *testing.T) {
	broker, err := NewBroker("localhost", 12345)
	require.NoError(t, err)
	err = broker.Start()
	require.NoError(t, err)
	
	broker.Stop()
	time.Sleep(100 * time.Millisecond) // Ensure Stop has time to complete

	select {
	case <-broker.stopChan:
		// Expected behavior: stopChan should be closed or signaled
	default:
		t.Errorf("Expected stopChan to be signaled")
	}
}

func TestHandlePacket(t *testing.T) {
	broker, err := NewBroker("localhost", 12345)
	require.NoError(t, err)
	err = broker.Start()
	require.NoError(t, err)

	subID := "test-sub"
	packetChan := broker.Sub(subID)
	packet := &rtp.Packet{
		// Add other necessary fields if required
	}
	broker.handleListeningRTP() // Run handleListeningRTP in a separate goroutine in a real test

	// Send a packet to simulate real behavior
	broker.subcriptions[subID] <- packet
	receivedPacket := <-packetChan
	assert.NotNil(t, receivedPacket)
	assert.Equal(t, packet.Version, receivedPacket.Version)
}

func TestCloneRTPPacket(t *testing.T) {
	packet := &rtp.Packet{
		// Add other necessary fields if required
	}

	clonedPacket := cloneRTPPacket(packet)
	assert.NotNil(t, clonedPacket)
	assert.Equal(t, packet.Version, clonedPacket.Version)
}

func TestDispose(t *testing.T) {
	broker, err := NewBroker("localhost", 12345)
	require.NoError(t, err)
	err = broker.Start()
	require.NoError(t, err)
	
	broker.Dispose()
	_, ok := <-broker.stopChan
	assert.False(t, ok, "Expected stopChan to be closed")
}
