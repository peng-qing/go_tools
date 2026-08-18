package encode_utils

import "testing"

func TestEncoderDecoderRoundTrip(t *testing.T) {
	encodings := []string{
		EncodingUTF8,
		EncodingUTF8BOM,
		EncodingGBK,
		EncodingGB18030,
		EncodingHZGB2312,
	}
	const input = "hello, world"
	for _, name := range encodings {
		t.Run(name, func(t *testing.T) {
			encoder := NewEncoder(name)
			decoder := NewDecoder(name)
			if encoder == nil || decoder == nil {
				t.Fatalf("encoder or decoder is nil for %q", name)
			}
			encoded, err := encoder.Bytes([]byte(input))
			if err != nil {
				t.Fatalf("encode %q: %v", name, err)
			}
			decoded, err := decoder.Bytes(encoded)
			if err != nil {
				t.Fatalf("decode %q: %v", name, err)
			}
			t.Logf("actual: %q", string(decoded))
			t.Logf("expected: %q", input)
			if string(decoded) != input {
				t.Fatalf("round trip %q = %q", name, decoded)
			}
		})
	}
}

func TestUnknownEncodingReturnsNil(t *testing.T) {
	actualEncoder := NewEncoder("unknown")
	t.Logf("actual: encoder=%v, decoder=%v", actualEncoder, NewDecoder("unknown"))
	t.Logf("expected: encoder=<nil>, decoder=<nil>")
	if actualEncoder != nil {
		t.Fatal("NewEncoder(unknown) did not return nil")
	}
	if NewDecoder("unknown") != nil {
		t.Fatal("NewDecoder(unknown) did not return nil")
	}
}
