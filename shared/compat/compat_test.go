package compat

import (
	"testing"
	"time"
)

func descriptors() (Descriptor, Descriptor) {
	host := NewDescriptor(
		"host", "", "", "test", "host-rev", "2026-07-28T00:00:00Z",
		HostCapabilities())
	firmware := NewDescriptor(
		"firmware", "zero-w-armhf", "device-1", "test", "pi-rev",
		"2026-07-28T00:00:00Z",
		FirmwareCapabilities(),
	)
	return host, firmware
}

func TestProtocolAndOperationCompatibility(t *testing.T) {
	host, firmware := descriptors()
	for _, operation := range []Operation{
		OperationWiiConnect, OperationGameCubePhysical, OperationGameCubeEmulated,
		OperationAutomaticSwitch, OperationReboot, OperationShutdown,
	} {
		result := Evaluate(host, firmware, EvaluateOptions{
			Operation: operation, ExpectedBoard: "zero-w-armhf",
			ExpectedDeviceID: "device-1",
		})
		if result.Status != StateCompatible || result.NegotiatedProtocol == nil {
			t.Fatalf("%s = %#v", operation, result)
		}
	}
}

func TestOverlappingAndNonOverlappingRanges(t *testing.T) {
	host, firmware := descriptors()
	host.ProtocolMin, host.ProtocolMax = 1, 3
	firmware.ProtocolMin, firmware.ProtocolMax = 2, 4
	result := Evaluate(host, firmware, EvaluateOptions{Operation: OperationStatus})
	if result.Status != StateCompatible || result.NegotiatedProtocol == nil ||
		*result.NegotiatedProtocol != 3 {
		t.Fatalf("overlap = %#v", result)
	}
	firmware.ProtocolMin = 4
	result = Evaluate(host, firmware, EvaluateOptions{Operation: OperationStatus})
	if result.Status != StateBlocked || len(result.Errors) == 0 ||
		result.Errors[0].Code != "COMPAT-PROTOCOL-NO-OVERLAP" {
		t.Fatalf("no overlap = %#v", result)
	}
}

func TestMissingRequiredAndOptionalCapabilities(t *testing.T) {
	host, firmware := descriptors()
	firmware.Capabilities = remove(firmware.Capabilities, CapGameCubeSaveOverlay)
	result := Evaluate(host, firmware, EvaluateOptions{Operation: OperationGameCubeEmulated})
	if result.Status != StateBlocked || len(result.MissingCapabilities) != 1 {
		t.Fatalf("missing required = %#v", result)
	}
	result = Evaluate(host, firmware, EvaluateOptions{Operation: OperationWiiConnect})
	if result.Status != StateCompatible {
		t.Fatalf("older firmware should retain Wii operation: %#v", result)
	}
	firmware.Capabilities = remove(firmware.Capabilities, CapRuntimeMetrics)
	result = Evaluate(host, firmware, EvaluateOptions{Operation: OperationPerformanceMetrics})
	if result.Status != StateCompatibleWithWarnings || len(result.Warnings) != 1 {
		t.Fatalf("missing optional = %#v", result)
	}
}

func TestMalformedSchemaAndIdentityMismatch(t *testing.T) {
	host, firmware := descriptors()
	firmware.SchemaVersion = 99
	result := Evaluate(host, firmware, EvaluateOptions{Operation: OperationStatus})
	if result.Status != StateBlocked ||
		result.Errors[0].Code != "COMPAT-DESCRIPTOR-MALFORMED" {
		t.Fatalf("schema = %#v", result)
	}
	_, firmware = descriptors()
	result = Evaluate(host, firmware, EvaluateOptions{
		Operation: OperationStatus, ExpectedDeviceID: "different",
	})
	if result.Status != StateBlocked ||
		result.Errors[0].Code != "COMPAT-DEVICE-IDENTITY-MISMATCH" {
		t.Fatalf("identity = %#v", result)
	}
	_, firmware = descriptors()
	unsupported := false
	result = Evaluate(host, firmware, EvaluateOptions{
		Operation: OperationWiiConnect, BoardSupported: &unsupported,
	})
	if result.Status != StateBlocked ||
		result.Errors[0].Code != "COMPAT-BOARD-UNSUPPORTED" {
		t.Fatalf("unsupported board = %#v", result)
	}
}

func TestMalformedBuildTimeIsRejected(t *testing.T) {
	host, firmware := descriptors()
	firmware.BuildTime = "not-a-time"
	result := Evaluate(host, firmware, EvaluateOptions{Operation: OperationStatus})
	if result.Status != StateBlocked ||
		result.Errors[0].Code != "COMPAT-DESCRIPTOR-MALFORMED" {
		t.Fatalf("build time = %#v", result)
	}
}

func TestUnknownOptionalCapabilityDoesNotBlock(t *testing.T) {
	host, firmware := descriptors()
	firmware.Capabilities = append(firmware.Capabilities, "future-observer-v9")
	result := Evaluate(host, firmware, EvaluateOptions{Operation: OperationWiiConnect})
	if result.Status != StateCompatible {
		t.Fatalf("unknown optional capability blocked: %#v", result)
	}
}

func TestUnknownRequiredCapabilityBlocksAndCachedResultBecomesStale(t *testing.T) {
	host, firmware := descriptors()
	result := Evaluate(host, firmware, EvaluateOptions{
		Operation:          OperationWiiConnect,
		AdditionalRequired: []string{"future-required-operation-v9"},
		Now:                time.Now().Add(-time.Minute),
	})
	if result.Status != StateBlocked ||
		len(result.MissingCapabilities) != 1 ||
		result.MissingCapabilities[0] != "future-required-operation-v9" {
		t.Fatalf("unknown required capability=%#v", result)
	}
	host, firmware = descriptors()
	result = Evaluate(host, firmware, EvaluateOptions{
		Operation: OperationWiiConnect, Now: time.Now().Add(-time.Minute),
	})
	stale := MarkCachedStale(result, time.Now(), 30*time.Second)
	if stale.Status != StateCompatibleWithWarnings ||
		len(stale.Warnings) != 1 || stale.Warnings[0].Code != "COMPAT-STATUS-STALE" {
		t.Fatalf("stale=%#v", stale)
	}
}

func remove(values []string, target string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		if value != target {
			result = append(result, value)
		}
	}
	return result
}
