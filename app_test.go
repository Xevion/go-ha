package gomeassistant

import (
	"context"
	"testing"
	"time"
)

func TestAppClose(t *testing.T) {
	// Create a new app with minimal configuration
	app := &App{
		ctx:       context.Background(),
		ctxCancel: func() {}, // No-op cancel function for test
	}

	// Test that Close() doesn't panic
	err := app.Close()
	if err != nil {
		t.Errorf("Close() returned error: %v", err)
	}
}

func TestAppCloseWithContext(t *testing.T) {
	// Create a context with cancel function
	ctx, cancel := context.WithCancel(context.Background())

	app := &App{
		ctx:       ctx,
		ctxCancel: cancel,
	}

	// Test that Close() cancels the context
	err := app.Close()
	if err != nil {
		t.Errorf("Close() returned error: %v", err)
	}

	// Verify context was cancelled
	select {
	case <-ctx.Done():
		// Context was cancelled as expected
	default:
		t.Error("Context was not cancelled by Close()")
	}
}

func TestAppCloseWithTimeout(t *testing.T) {
	// Create a context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	app := &App{
		ctx:       ctx,
		ctxCancel: cancel,
	}

	// Test that Close() works with timeout context
	err := app.Close()
	if err != nil {
		t.Errorf("Close() returned error: %v", err)
	}
}

func TestAppCleanup(t *testing.T) {
	// Test the legacy Cleanup method
	ctx, cancel := context.WithCancel(context.Background())

	app := &App{
		ctx:       ctx,
		ctxCancel: cancel,
	}

	// Test that Cleanup() doesn't panic
	app.Cleanup()

	// Verify context was cancelled
	select {
	case <-ctx.Done():
		// Context was cancelled as expected
	default:
		t.Error("Context was not cancelled by Cleanup()")
	}
}

func TestAppGetService(t *testing.T) {
	// Test GetService method
	app := &App{
		service: &Service{},
	}

	service := app.GetService()
	if service == nil {
		t.Error("GetService() returned nil")
	}
}

func TestAppGetState(t *testing.T) {
	// Test GetState method
	app := &App{
		state: &StateImpl{},
	}

	state := app.GetState()
	if state == nil {
		t.Error("GetState() returned nil")
	}
}

func TestAppWithNilFields(t *testing.T) {
	// Test app with nil fields to ensure no panics
	app := &App{}

	// Test Close with nil fields
	err := app.Close()
	if err != nil {
		t.Errorf("Close() returned error: %v", err)
	}

	// Test Cleanup with nil fields
	app.Cleanup()

	// Test GetService with nil service
	service := app.GetService()
	if service != nil {
		t.Error("GetService() should return nil when service is nil")
	}

	// Test GetState with nil state
	state := app.GetState()
	// When state is nil, GetState returns a typed nil (*StateImpl)
	// This is the correct behavior - the interface is not nil but the value is nil
	_ = state // Just ensure it doesn't panic
}

func TestAppWithWebsocketConnection(t *testing.T) {
	// Test app with WebSocket connection (mocked)
	app := &App{
		ctx:       context.Background(),
		ctxCancel: func() {},
		conn:      nil, // In real test, this would be a mock WebSocket
	}

	// Test that Close() handles nil connection gracefully
	err := app.Close()
	if err != nil {
		t.Errorf("Close() returned error: %v", err)
	}
}

func TestAppRegisterMethods(t *testing.T) {
	// Test that register methods don't panic with empty app
	app := &App{
		entityListeners: make(map[string][]*EntityListener),
		eventListeners:  make(map[string][]*EventListener),
	}

	// Test registering empty schedules
	app.RegisterSchedules()

	// Test registering empty intervals
	app.RegisterIntervals()

	// Test registering empty entity listeners
	app.RegisterEntityListeners()

	// Test registering empty event listeners
	app.RegisterEventListeners()
}

func TestAppContextCancellation(t *testing.T) {
	// Test that context cancellation works properly
	ctx, cancel := context.WithCancel(context.Background())

	app := &App{
		ctx:       ctx,
		ctxCancel: cancel,
	}

	// Cancel context manually
	cancel()

	// Verify context is done
	select {
	case <-ctx.Done():
		// Expected
	default:
		t.Error("Context should be cancelled")
	}

	// Test Close after manual cancellation
	err := app.Close()
	if err != nil {
		t.Errorf("Close() returned error: %v", err)
	}
}
