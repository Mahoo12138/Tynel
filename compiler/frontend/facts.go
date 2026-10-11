// Package frontend owns the scalar frontend contract and lowering. It imports
// neither tsgo nor Go compiler internals. Facts are data, not checker handles.
package frontend

import "fmt"

const Version = 2
const Profile = "tynel.scalar/2"

type Span struct {
	File      string `json:"file"`
	StartByte int    `json:"startByte"`
	EndByte   int    `json:"endByte"`
	Line      int    `json:"line"`
	Column    int    `json:"utf16Column"`
}

type Binding struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Type string `json:"type"`
	Loc  Span   `json:"loc"`
}

type Function struct {
	ID     string      `json:"id"`
	Name   string      `json:"name"`
	Params []Binding   `json:"params"`
	Result string      `json:"result"`
	Body   []Statement `json:"body"`
	Loc    Span        `json:"loc"`
}

type Expression struct {
	Op   string        `json:"op"`
	Type string        `json:"type"`
	Ref  string        `json:"ref,omitempty"`
	Bits string        `json:"bits,omitempty"`
	Bool *bool         `json:"bool,omitempty"`
	Args []*Expression `json:"args,omitempty"`
	Loc  Span          `json:"loc"`
}

type Statement struct {
	Op        string      `json:"op"`
	Binding   *Binding    `json:"binding,omitempty"`
	Value     *Expression `json:"value"`
	Condition *Expression `json:"condition,omitempty"`
	Then      []Statement `json:"then,omitempty"`
	Else      []Statement `json:"else,omitempty"`
	Body      []Statement `json:"body,omitempty"`
	Loc       Span        `json:"loc"`
}

type Program struct {
	Version      int         `json:"version"`
	Profile      string      `json:"profile"`
	SourceSHA256 string      `json:"sourceSHA256"`
	File         string      `json:"file"`
	Functions    []Function  `json:"functions"`
	Body         []Statement `json:"body"`
}

// Error reports a native capability/contract diagnostic, distinct from TS errors.
type Error struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Loc     Span   `json:"loc"`
}

func (e Error) Error() string {
	return fmt.Sprintf("%s:%d:%d: %s: %s", e.Loc.File, e.Loc.Line, e.Loc.Column, e.Code, e.Message)
}
