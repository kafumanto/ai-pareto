package cache

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"
)

const (
	// maxBodyBytes is the largest complete response body accepted by cache policy.
	maxBodyBytes int64 = 64 * 1024 * 1024
	// cacheReadOperation labels recoverable storage-read warnings.
	cacheReadOperation = "read"
	// cacheWriteOperation labels recoverable storage-write warnings.
	cacheWriteOperation = "write"
)

// policy applies cache identity, freshness, response, and warning rules to one Store.
//
// A policy does not construct HTTP requests. Callers provide deferred network
// execution through Request and retain ownership of source-specific authentication.
type policy struct {
	// store persists complete raw response entries under opaque keys.
	store Store
	// now supplies the current time for freshness and entry creation.
	now func() time.Time
}

// NewRetriever returns a cache-aware retriever using the system clock for freshness decisions.
//
// The returned retriever uses store for all cache reads and writes. A nil store is
// invalid for cacheable GET requests but still permits non-GET network bypasses.
func NewRetriever(store Store) Retriever {
	return newPolicy(store, time.Now)
}

// newPolicy constructs a policy with an injectable clock for deterministic tests.
//
// A nil clock selects time.Now so the policy remains usable when callers only
// need the default production behavior.
func newPolicy(store Store, now func() time.Time) *policy {
	// Fall back to the production clock when callers do not need deterministic time control.
	if now == nil {
		now = time.Now
	}
	return &policy{store: store, now: now}
}

// Retrieve returns a fresh cached response or performs deferred network retrieval.
//
// The method reads and closes live response bodies, stores only complete
// successful GET bodies, and returns recoverable storage failures as warnings.
// It returns a context error when cancellation or deadline expiry occurs.
func (policy *policy) Retrieve(ctx context.Context, request Request) (Result, error) {
	// Stop immediately when the caller has already cancelled the cache operation.
	if contextErr := cacheContextError(ctx, nil); contextErr != nil {
		return Result{}, contextErr
	}
	// Require a positive TTL so freshness always has a valid comparison window.
	if request.TTL <= 0 {
		return Result{}, errors.New("cache request TTL must be positive")
	}

	warnings := make([]Warning, 0, 1)
	key := ""
	if request.Method == http.MethodGet {
		// GET requests need storage for both reads and successful replacements.
		if policy.store == nil {
			return Result{}, errors.New("cache store is nil")
		}
		key = keyFor(request)
	}

	// Refresh and non-GET requests bypass storage reads and proceed directly to live execution.
	if request.Method == http.MethodGet && !request.Refresh {
		entry, found, err := policy.store.Get(ctx, key)
		// Preserve cancellation instead of downgrading a cancelled read to a warning.
		if contextErr := cacheContextError(ctx, err); contextErr != nil {
			return Result{}, contextErr
		}
		// Continue to live execution after a recoverable read failure and retain one warning.
		if err != nil {
			warnings = append(warnings, Warning{Operation: cacheReadOperation, Err: err})
		} else if found && policy.now().Sub(entry.CreatedAt) < request.TTL {
			// Return a strictly fresh entry without invoking deferred network execution.
			return Result{
				Body:       cloneBytes(entry.Body),
				StatusCode: http.StatusOK,
				FromCache:  true,
				Warnings:   warnings,
			}, nil
		}
	}

	// Check cancellation again after storage work before invoking the network.
	if contextErr := cacheContextError(ctx, nil); contextErr != nil {
		return Result{}, contextErr
	}
	// A miss cannot proceed without the caller's deferred network function.
	if request.Network == nil {
		return Result{Warnings: warnings}, errors.New("cache request has no deferred network")
	}

	response, err := request.Network(ctx)
	// Preserve context errors returned by the deferred network before handling ordinary failures.
	if contextErr := cacheContextError(ctx, err); contextErr != nil {
		return Result{}, contextErr
	}
	if err != nil {
		// A non-nil response with a transport error still owns a body that must be closed.
		if response != nil && response.Body != nil {
			_ = response.Body.Close()
		}
		return Result{Warnings: warnings}, err
	}
	// Reject an invalid successful callback result before dereferencing its body.
	if response == nil {
		return Result{Warnings: warnings}, errors.New("deferred network returned a nil response")
	}

	body, err := readResponseBody(ctx, response)
	if err != nil {
		return Result{}, err
	}
	result := Result{
		Body:       body,
		StatusCode: response.StatusCode,
		Warnings:   warnings,
	}

	// Only GET success responses reach storage; all other live responses remain available to the caller.
	if request.Method != http.MethodGet || response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return result, nil
	}

	// Store one complete body with the creation time observed after reading it.
	entry := Entry{CreatedAt: policy.now(), Body: cloneBytes(body)}
	if err := policy.store.Put(ctx, key, entry); err != nil {
		// Preserve cancellation instead of reporting a write warning.
		if contextErr := cacheContextError(ctx, err); contextErr != nil {
			return Result{}, contextErr
		}
		result.Warnings = append(result.Warnings, Warning{Operation: cacheWriteOperation, Err: err})
		return result, nil
	}
	// A store that cancels after a successful write still cancels the operation.
	if contextErr := cacheContextError(ctx, nil); contextErr != nil {
		return Result{}, contextErr
	}
	return result, nil
}

// readResponseBody reads one complete bounded response body and always closes it.
//
// The returned body is nil on every error, including an oversized or incomplete
// body, so callers cannot accidentally cache or expose partial response bytes.
func readResponseBody(ctx context.Context, response *http.Response) (body []byte, err error) {
	// A response without a body cannot satisfy the complete-body contract.
	if response.Body == nil {
		return nil, errors.New("HTTP response body is nil")
	}
	defer func() {
		// Close the live body on every return path and retain the first relevant failure.
		closeErr := response.Body.Close()
		if err != nil {
			body = nil
			return
		}
		if contextErr := cacheContextError(ctx, closeErr); contextErr != nil {
			body = nil
			err = contextErr
			return
		}
		if closeErr != nil {
			body = nil
			err = closeErr
		}
	}()

	// Check cancellation before starting a potentially blocking body read.
	if contextErr := cacheContextError(ctx, nil); contextErr != nil {
		return nil, contextErr
	}
	// Read one extra byte so an oversized body is rejected without returning a truncated body.
	body, err = io.ReadAll(io.LimitReader(response.Body, maxBodyBytes+1))
	if err != nil {
		return nil, err
	}
	if contextErr := cacheContextError(ctx, nil); contextErr != nil {
		return nil, contextErr
	}
	// Reject bodies that exceed the policy limit before any cache write.
	if int64(len(body)) > maxBodyBytes {
		return nil, fmt.Errorf("HTTP response body exceeds %d bytes", maxBodyBytes)
	}
	// Reject a body that ends before a nonnegative declared content length.
	if response.ContentLength >= 0 && int64(len(body)) < response.ContentLength {
		return nil, fmt.Errorf("HTTP response body ended at %d bytes, declared %d", len(body), response.ContentLength)
	}
	return body, nil
}

// cacheContextError gives cancellation precedence over cache warnings and other failures.
func cacheContextError(ctx context.Context, err error) error {
	// The live context takes precedence over any storage or network error.
	if contextErr := ctx.Err(); contextErr != nil {
		return contextErr
	}
	// Preserve explicit cancellation identities returned by dependencies.
	if errors.Is(err, context.Canceled) {
		return context.Canceled
	}
	// Preserve explicit deadline identities returned by dependencies.
	if errors.Is(err, context.DeadlineExceeded) {
		return context.DeadlineExceeded
	}
	return nil
}

// cloneBytes preserves nil versus empty bodies while preventing caller mutation of stored bytes.
func cloneBytes(body []byte) []byte {
	// Preserve nil so callers can distinguish an absent body from a valid empty body.
	if body == nil {
		return nil
	}
	// Copy bytes so source decoding cannot mutate the stored entry.
	clone := make([]byte, len(body))
	copy(clone, body)
	return clone
}
