package ha

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

func TestAppServices(t *testing.T) {
	// Test Services method
	app := &App{
		service: &Service{},
	}

	service := app.Services()
	if service == nil {
		t.Error("Services() returned nil")
	}
}

func TestAppState(t *testing.T) {
	// Test State method
	app := &App{
		state: &state{},
	}

	s := app.State()
	if s == nil {
		t.Error("State() returned nil")
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

	// Test Services with nil service
	service := app.Services()
	if service != nil {
		t.Error("Services() should return nil when service is nil")
	}

	// When the concrete state is nil, State returns a typed nil, so the
	// interface is non-nil while the value inside it is not. Only assert that
	// reaching for it does not panic.
	_ = app.State()
}

func TestAppWithWebsocketConnection(t *testing.T) {
	app := &App{
		ctx:       context.Background(),
		ctxCancel: func() {},
		client:    nil,
	}

	// Test that Close() handles nil connection gracefully
	err := app.Close()
	if err != nil {
		t.Errorf("Close() returned error: %v", err)
	}
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
