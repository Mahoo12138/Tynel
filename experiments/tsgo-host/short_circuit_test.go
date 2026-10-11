package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"tynel.local/tynel/compiler/frontend"
)

func TestShortCircuitSourceForms(t *testing.T) {
	cases := map[string]string{
		"and":                "function f(a:boolean,b:boolean):boolean{return a&&b;}console.log(f(true,false));",
		"or":                 "function f(a:boolean,b:boolean):boolean{return a||b;}console.log(f(false,true));",
		"nested":             "function f(a:boolean,b:boolean,c:boolean):boolean{return a&&(b||c);}console.log(f(true,false,true));",
		"precedence":         "function f(a:boolean,b:boolean,c:boolean):boolean{return a||b&&c;}console.log(f(true,false,false));",
		"condition":          "function f(a:boolean,b:boolean):number{if(a&&b)return 1;return 2;}console.log(f(true,false));",
		"comparison":         "function f(x:number,y:number):boolean{return x>0&&y<=0;}console.log(f(1,-1));",
		"false-left":         "function f():boolean{console.log(99);return true;}console.log(false&&f());",
		"true-left":          "function f():boolean{console.log(99);return false;}console.log(true||f());",
		"const-and-argument": "function f(x:boolean):boolean{return x;}const b=false||true;console.log(f(b&&true));",
		"discard":            "function f():boolean{console.log(1);return true;}false&&f();true||f();",
		"flow-narrowing":     "function f(a:boolean):boolean{return a&&(a===true);}console.log(f(false));",
		"unicode":            "function 判断(值:boolean):boolean{return 值&&true;}console.log(判断(false));",
	}
	for name, source := range cases {
		t.Run(name, func(t *testing.T) {
			p := extract(t, source)
			if _, err := frontend.Lower(p); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestShortCircuitSourceRejections(t *testing.T) {
	cases := map[string]string{
		"numeric-left":           "function f(x:number):boolean{return x&&true;}console.log(f(1));",
		"hidden-number-right":    "console.log(false&&42);",
		"hidden-number-right-or": "console.log(true||42);",
		"numeric-values":         "function f(x:number,y:number):number{return x||y;}console.log(f(0,2));",
		"skipped-remainder":      "console.log(false&&((1%2)>0));",
		"skipped-library":        "console.log(true||(Math.abs(-1)>0));",
		"skipped-assertion":      "console.log(false&&(1 as any));",
		"unused-function":        "function f(a:boolean):boolean{return a&&((1%2)>0);}console.log(42);",
		"nullish":                "function f(a:boolean):boolean{return a??false;}console.log(f(true));",
		"logical-assignment":     "function f(a:boolean,b:boolean):boolean{a&&=b;return a;}console.log(f(true,false));",
		"or-assignment":          "function f(a:boolean,b:boolean):boolean{a||=b;return a;}console.log(f(true,false));",
		"skipped-unknown":        "console.log(false&&missing);",
	}
	for name, source := range cases {
		t.Run(name, func(t *testing.T) {
			var out, errs bytes.Buffer
			code := emitScalar(source, "short-negative.ts", "nir", &out, &errs)
			if code == 0 || out.Len() != 0 || (!strings.Contains(errs.String(), "TY12") && !strings.Contains(errs.String(), "TS")) {
				t.Fatalf("exit=%d out=%s errs=%s", code, &out, &errs)
			}
		})
	}
}

func TestShortCircuitAfterSessionClose(t *testing.T) {
	p := extract(t, "function f(a:boolean,b:boolean):boolean{return a&&b;}console.log(f(false,true));")
	if p.Functions[0].Body[0].Value.Op != "and" {
		t.Fatal("logical node missing")
	}
	data, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	var q frontend.Program
	if err = json.Unmarshal(data, &q); err != nil {
		t.Fatal(err)
	}
	if _, err = frontend.Lower(&q); err != nil {
		t.Fatal(err)
	}
}

func TestShortCircuitDepthRejected(t *testing.T) {
	source := "function f(a:boolean):boolean{return " + strings.Repeat("a&&(", 75) + "a" + strings.Repeat(")", 75) + ";}"
	var out, errs bytes.Buffer
	if code := emitScalar(source, "deep.ts", "nir", &out, &errs); code == 0 || out.Len() != 0 || !strings.Contains(errs.String(), "budget") {
		t.Fatalf("exit=%d errs=%s", code, &errs)
	}
}
