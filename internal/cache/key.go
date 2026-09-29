package cache

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"sort"
	"strings"
)

const (
	// keyNamespaceField identifies the source namespace in the framed identity.
	keyNamespaceField byte = 1
	// keyMethodField identifies the HTTP method in the framed identity.
	keyMethodField byte = 2
	// keyURLField identifies the exact request URL in the framed identity.
	keyURLField byte = 3
	// keyHeaderNameField identifies one canonical header name in the framed identity.
	keyHeaderNameField byte = 4
	// keyHeaderValueField identifies one header value in the framed identity.
	keyHeaderValueField byte = 5
	// keyCredentialAbsentField identifies a request without a selected credential.
	keyCredentialAbsentField byte = 6
	// keyCredentialDigestField identifies a request with a hashed credential.
	keyCredentialDigestField byte = 7
)

// canonicalHeader stores one header name and its original value order for key encoding.
//
// Header names are normalized before sorting, while values remain ordered within
// each input header entry because some representations distinguish value order.
type canonicalHeader struct {
	// name is the lowercase HTTP header name used for identity.
	name string
	// values contains the header values in the caller-provided order.
	values []string
}

// keyFor derives the opaque deterministic identity for one cache request.
//
// The result is a lowercase SHA-256 digest of length-framed fields. Authorization
// headers are excluded, and a selected credential contributes only its digest.
// The function does not normalize the namespace, method, or URL values.
func keyFor(request Request) string {
	var encoded bytes.Buffer
	// Encode the fields whose exact values identify the source representation.
	appendKeyField(&encoded, keyNamespaceField, []byte(request.Namespace))
	appendKeyField(&encoded, keyMethodField, []byte(request.Method))
	appendKeyField(&encoded, keyURLField, []byte(request.URL))

	// Normalize names and retain each value slice so map iteration cannot affect the key.
	headers := make([]canonicalHeader, 0, len(request.Headers))
	for name, values := range request.Headers {
		// Authorization is request authentication, not a representation input, so exclude it from identity.
		if strings.EqualFold(name, "Authorization") {
			continue
		}
		headers = append(headers, canonicalHeader{
			name:   strings.ToLower(name),
			values: append([]string(nil), values...),
		})
	}

	// Sort by normalized name and then values to make duplicate differently-cased map keys deterministic.
	sort.Slice(headers, func(i, j int) bool {
		// Compare header names first so map iteration order cannot change field order.
		if headers[i].name != headers[j].name {
			return headers[i].name < headers[j].name
		}
		// Compare duplicate-name value sequences to keep unusual mixed-case maps deterministic.
		return compareHeaderValues(headers[i].values, headers[j].values) < 0
	})
	for _, header := range headers {
		// Encode the canonical name before its ordered values.
		appendKeyField(&encoded, keyHeaderNameField, []byte(header.name))
		for _, value := range header.values {
			// Preserve caller-provided value order within one header entry.
			appendKeyField(&encoded, keyHeaderValueField, []byte(value))
		}
	}

	// Mark credential absence separately so an empty selected credential differs from no credential.
	if request.Credential == nil {
		appendKeyField(&encoded, keyCredentialAbsentField, nil)
	} else {
		credentialDigest := sha256.Sum256(request.Credential)
		appendKeyField(&encoded, keyCredentialDigestField, credentialDigest[:])
	}

	digest := sha256.Sum256(encoded.Bytes())
	return hex.EncodeToString(digest[:])
}

// compareHeaderValues compares two value slices lexicographically for deterministic duplicate-name ordering.
func compareHeaderValues(left, right []string) int {
	// Compare only the shared prefix before comparing slice lengths.
	limit := len(left)
	if len(right) < limit {
		limit = len(right)
	}
	for index := 0; index < limit; index++ {
		// Return immediately when one corresponding value sorts first.
		if left[index] < right[index] {
			return -1
		}
		if left[index] > right[index] {
			return 1
		}
	}
	// A shared prefix sorts before a longer sequence.
	if len(left) < len(right) {
		return -1
	}
	if len(left) > len(right) {
		return 1
	}
	return 0
}

// appendKeyField appends one marker, byte length, and value to a key serialization.
func appendKeyField(encoded *bytes.Buffer, marker byte, value []byte) {
	encoded.WriteByte(marker)
	var length [8]byte
	// Encode every length with a fixed-width big-endian value for architecture-independent framing.
	binary.BigEndian.PutUint64(length[:], uint64(len(value)))
	encoded.Write(length[:])
	encoded.Write(value)
}
