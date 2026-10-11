package nir

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
)

const MaxArtifactBytes = 1 << 20

func Read(r io.Reader) (*Module, error) {
	data, err := io.ReadAll(io.LimitReader(r, MaxArtifactBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > MaxArtifactBytes {
		return nil, fmt.Errorf("IR exceeds %d bytes", MaxArtifactBytes)
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	var m Module
	if err := dec.Decode(&m); err != nil {
		return nil, fmt.Errorf("decode IR: %w", err)
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		return nil, fmt.Errorf("trailing data after IR module")
	}
	if err := Validate(&m); err != nil {
		return nil, err
	}
	return &m, nil
}

func Write(w io.Writer, m *Module) error {
	if err := Validate(m); err != nil {
		return err
	}
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if len(data) > MaxArtifactBytes {
		return fmt.Errorf("serialized IR exceeds %d bytes", MaxArtifactBytes)
	}
	n, err := w.Write(data)
	if err == nil && n != len(data) {
		return io.ErrShortWrite
	}
	return err
}
