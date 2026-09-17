package network

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"testing"
)

const (
	testMaxPayload = (4 << 20) - LtvHeaderSize
)

// TestLtvCodecRoundTrip 验证数据包编码后可以完整解码，类型和负载内容均保持不变。
func TestLtvCodecRoundTrip(t *testing.T) {
	tests := []struct {
		name         string
		littleEndian bool
	}{
		{name: "big endian", littleEndian: false},
		{name: "little endian", littleEndian: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			codec := NewLtvCodec(testMaxPayload, tt.littleEndian)
			expected := NewLtvPacket(0x01020304, []byte("hello LTV"))
			encoded, err := codec.Encode(expected)
			if err != nil {
				t.Fatalf("Encode() error = %v", err)
			}

			actual, consumed, err := codec.Decode(encoded)
			t.Logf("actual: packet=%#v, consumed=%d, err=%v", actual, consumed, err)
			t.Logf("expected: packet=%#v, consumed=%d, err=<nil>", expected, len(encoded))

			if err != nil {
				t.Fatalf("Decode() error = %v", err)
			}
			if consumed != len(encoded) {
				t.Fatalf("Decode() consumed = %d, want %d", consumed, len(encoded))
			}
			if actual == nil || actual.Type != expected.Type ||
				!bytes.Equal(actual.Payload, expected.Payload) {
				t.Fatalf("Decode() packet = %#v, want %#v", actual, expected)
			}
		})
	}
}

// TestLtvCodecPartialPacketDoesNotReportPacket 验证包头不完整或负载不完整时，不会误报为完整数据包。
func TestLtvCodecPartialPacketDoesNotReportPacket(t *testing.T) {
	codec := NewLtvCodec(testMaxPayload, false)
	encoded, err := codec.Encode(NewLtvPacket(7, []byte("fragmented payload")))
	if err != nil {
		t.Fatalf("Encode() error = %v", err)
	}

	cuts := []int{0, 1, LtvHeaderSize - 1, LtvHeaderSize, len(encoded) - 1}
	for _, cut := range cuts {
		t.Run(fmt.Sprintf("length_%d", cut), func(t *testing.T) {
			actual, consumed, err := codec.Decode(encoded[:cut])
			t.Logf("actual: input_length=%d, packet=%#v, consumed=%d, err=%v", cut, actual, consumed, err)
			t.Logf("expected: packet=<nil>, consumed=0, err=<nil>")

			if err != nil || actual != nil || consumed != 0 {
				t.Fatalf(
					"Decode(partial length %d) = packet:%#v consumed:%d err:%v",
					cut,
					actual,
					consumed,
					err,
				)
			}
		})
	}
}

// TestLtvCodecStickyPacketsDecodedOneAtATime 验证多个数据包粘连时，可以按照原始顺序逐个解码。
func TestLtvCodecStickyPacketsDecodedOneAtATime(t *testing.T) {
	codec := NewLtvCodec(testMaxPayload, false)
	expected := []*LtvPacket{
		NewLtvPacket(1, []byte("first")),
		NewLtvPacket(2, []byte("second")),
		NewLtvPacket(3, []byte("third packet")),
	}

	stream := make([]byte, 0)
	encodedLengths := make([]int, 0, len(expected))
	for _, packet := range expected {
		encoded, err := codec.Encode(packet)
		if err != nil {
			t.Fatalf("Encode(%#v) error = %v", packet, err)
		}
		stream = append(stream, encoded...)
		encodedLengths = append(encodedLengths, len(encoded))
	}

	offset := 0
	for i, want := range expected {
		actual, consumed, err := codec.Decode(stream[offset:])
		t.Logf("actual[%d]: packet=%#v, consumed=%d, err=%v", i, actual, consumed, err)
		t.Logf("expected[%d]: packet=%#v, consumed=%d, err=<nil>", i, want, encodedLengths[i])

		if err != nil {
			t.Fatalf("Decode(packet %d) error = %v", i, err)
		}
		if consumed != encodedLengths[i] {
			t.Fatalf("Decode(packet %d) consumed = %d, want %d", i, consumed, encodedLengths[i])
		}
		if actual == nil || actual.Type != want.Type ||
			!bytes.Equal(actual.Payload, want.Payload) {
			t.Fatalf("Decode(packet %d) = %#v, want %#v", i, actual, want)
		}
		offset += consumed
	}
	if offset != len(stream) {
		t.Fatalf("decoded bytes = %d, stream length = %d", offset, len(stream))
	}
}

// TestLtvCodecOversizedLengthDoesNotAllocate 验证恶意超大长度在分配负载内存前即被拒绝，避免异常内存分配。
func TestLtvCodecOversizedLengthDoesNotAllocate(t *testing.T) {
	codec := NewLtvCodec(64, false)
	header := make([]byte, LtvHeaderSize)
	binary.BigEndian.PutUint32(header[:4], math.MaxUint32)
	binary.BigEndian.PutUint32(header[4:8], 99)

	var (
		actual   *LtvPacket
		consumed int
		err      error
	)
	allocations := testing.AllocsPerRun(1000, func() {
		actual, consumed, err = codec.Decode(header)
	})

	t.Logf(
		"actual: packet=%#v, consumed=%d, err=%v, allocations_per_decode=%.0f",
		actual,
		consumed,
		err,
		allocations,
	)
	t.Logf(
		"expected: packet=<nil>, consumed=0, err=%v, allocations_per_decode=0",
		ErrPacketTooLarge,
	)

	if actual != nil {
		t.Fatalf("Decode(oversized) packet = %#v, want nil", actual)
	}
	if consumed != 0 {
		t.Fatalf("Decode(oversized) consumed = %d, want 0", consumed)
	}
	if !errors.Is(err, ErrPacketTooLarge) {
		t.Fatalf("Decode(oversized) error = %v, want %v", err, ErrPacketTooLarge)
	}
	if allocations != 0 {
		t.Fatalf("Decode(oversized) allocations = %.0f, want 0", allocations)
	}
}

// TestLtvCodecEncodePayloadBoundary 验证最大允许长度可以编码，超过限制时返回 ErrPacketTooLarge。
func TestLtvCodecEncodePayloadBoundary(t *testing.T) {
	codec := NewLtvCodec(4, false)
	acceptedPayload := []byte{1, 2, 3, 4}
	accepted, acceptedErr := codec.Encode(NewLtvPacket(1, acceptedPayload))
	rejected, rejectedErr := codec.Encode(NewLtvPacket(2, []byte{1, 2, 3, 4, 5}))

	t.Logf("actual: accepted_length=%d, accepted_err=%v, rejected=%v, rejected_err=%v", len(accepted), acceptedErr, rejected, rejectedErr)
	t.Logf("expected: accepted_length=%d, accepted_err=<nil>, rejected=[], rejected_err=%v", LtvHeaderSize+len(acceptedPayload), ErrPacketTooLarge)

	if acceptedErr != nil || len(accepted) != LtvHeaderSize+len(acceptedPayload) {
		t.Fatalf("maximum payload Encode() = length:%d err:%v", len(accepted), acceptedErr)
	}
	if rejected != nil || !errors.Is(rejectedErr, ErrPacketTooLarge) {
		t.Fatalf("oversized Encode() = data:%v err:%v", rejected, rejectedErr)
	}
}

// TestLtvCodecPayloadMemoryIndependent 验证数据包和解码结果不会共享调用方的输入缓冲区。
func TestLtvCodecPayloadMemoryIndependent(t *testing.T) {
	codec := NewLtvCodec(testMaxPayload, false)
	source := []byte("owned payload")
	packet := NewLtvPacket(7, source)
	source[0] = 'X'

	encoded, err := codec.Encode(packet)
	if err != nil {
		t.Fatalf("Encode() error = %v", err)
	}
	decoded, _, err := codec.Decode(encoded)
	if err != nil {
		t.Fatalf("Decode() error = %v", err)
	}
	encoded[LtvHeaderSize] = 'Y'

	t.Logf("actual: packet_payload=%q, decoded_payload=%q", packet.Payload, decoded.Payload)
	t.Logf("expected: packet_payload=%q, decoded_payload=%q", "owned payload", "owned payload")

	if string(packet.Payload) != "owned payload" {
		t.Fatalf("packet payload changed with source buffer: %q", packet.Payload)
	}
	if string(decoded.Payload) != "owned payload" {
		t.Fatalf("decoded payload changed with encoded buffer: %q", decoded.Payload)
	}
}
