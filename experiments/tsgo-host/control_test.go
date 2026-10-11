package main

import (
	"bytes"
	"strings"
	"testing"
	"tynel.local/tynel/compiler/frontend"
)

func TestControlAcceptedForms(t *testing.T) {
	cases := map[string]string{
		"boolean-signature":      "function inv(x:boolean):boolean{return !x;} console.log(inv(false));",
		"boolean-literals":       "const a=true; const b:boolean=false; console.log(a);console.log(b);",
		"complete-if":            "function f(x:number):number{if(x>0){return x;}else{return -x;}} console.log(f(-2));",
		"early-return":           "function f(x:number):number{if(x>0)return x;return 0;} console.log(f(0));",
		"else-if":                "function f(x:number):number{if(x<0)return -1;else if(x>0)return 1;else return 0;} console.log(f(0));",
		"boolean-equality":       "function same(a:boolean,b:boolean):boolean{return a===b;} console.log(same(false,true));",
		"numeric-equality":       "function same(a:number,b:number):boolean{return a!==b;} console.log(same(0/0,0/0));",
		"ordered-comparisons":    "function f(a:number,b:number):boolean{if(a<=b)return a<b;return a>=b;} console.log(f(1,2));",
		"block-scope":            "const x=1;{const x=2;console.log(x);}console.log(x);",
		"shadowed-branches":      "function f(x:number):number{if(x>0){const x=41;return x+1;}else{const x=0;return x;}}console.log(f(1));",
		"top-level-branch":       "if(false){console.log(1);}else{console.log(2);}",
		"empty-branches":         "if(true){}else{};if(false){}console.log(42);",
		"return-in-block":        "function f():number{{const x=42;return x;}}console.log(f());",
		"boolean-call-condition": "function flag(x:boolean):boolean{console.log(x);return x;}if(flag(true)){console.log(42);}",
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
func TestControlRejectedWithoutArtifact(t *testing.T) {
	cases := map[string]string{
		"numeric-condition":        "if(1)console.log(42);",
		"numeric-not":              "console.log(!0);",
		"boolean-unary-plus":       "console.log(+true);",
		"boolean-order":            "function f(a:boolean,b:boolean):boolean{return a<b;}console.log(f(false,true));",
		"loose-equality":           "function f(a:number,b:number):boolean{return a==b;}console.log(f(1,2));",
		"and":                      "function f(a:boolean,b:boolean):boolean{return a&&b;}console.log(f(true,false));",
		"or":                       "function f(a:boolean,b:boolean):boolean{return a||b;}console.log(f(true,false));",
		"ternary":                  "function f(b:boolean):number{return b?1:2;}console.log(f(true));",
		"false-branch-unsupported": "if(false){console.log(1%2);}console.log(42);",
		"nested-function":          "if(true){function f():number{return 1;}console.log(f());}",
		"missing-return":           "function f(b:boolean):number{if(b)return 1;}",
		"constant-is-not-proof":    "function f():number{if(true)return 1;}",
		"after-complete-if":        "function f(b:boolean):number{if(b)return 1;else return 2;console.log(3);}",
		"branch-leak":              "if(true){const hidden=1;}console.log(hidden);",
		"uninitialized-shadow":     "const x=1;if(true){console.log(x);const x=2;}",
		"wrong-return-type":        "function f():boolean{return 1;}",
		"wrong-call-type":          "function f(x:boolean):boolean{return x;}console.log(f(1));",
		"boolean-object":           "const x=new Boolean(false);if(x)console.log(1);",
		"union-parameter":          "function f(x:number|boolean):boolean{return x===1;}console.log(f(1));",
		"assignment":               "function f(x:boolean):boolean{x=false;return x;}console.log(f(true));",
	}
	for name, source := range cases {
		t.Run(name, func(t *testing.T) {
			var out, errs bytes.Buffer
			code := emitScalar(source, "rejected.ts", "nir", &out, &errs)
			if code == 0 || out.Len() != 0 || (!strings.Contains(errs.String(), "TY12") && !strings.Contains(errs.String(), "TS")) {
				t.Fatalf("exit=%d stdout=%s stderr=%s", code, &out, &errs)
			}
		})
	}
}
func TestBranchProjectionHasDistinctShadowSymbols(t *testing.T) {
	p := extract(t, "function f(x:number):number{if(x>0){const x=41;return x;}else{const x=0;return x;}}console.log(f(1));")
	f := p.Functions[0]
	branch := f.Body[0]
	a, b := branch.Then[0].Binding, branch.Else[0].Binding
	if a.ID == b.ID || a.ID == f.Params[0].ID || branch.Then[1].Value.Ref != a.ID || branch.Else[0].Binding.Type != "number" {
		t.Fatal("shadowed branch identities merged")
	}
}
func TestControlFalseBoolRemainsPresent(t *testing.T) {
	p := extract(t, "const flag:boolean=false;console.log(flag);")
	e := p.Body[0].Value
	if e.Type != "boolean" || e.Bool == nil || *e.Bool {
		t.Fatal("false lost in checker projection")
	}
}
func TestControlDepthRejected(t *testing.T) {
	source := strings.Repeat("if(true){", 70) + "console.log(42);" + strings.Repeat("}", 70)
	var out, errs bytes.Buffer
	if code := emitScalar(source, "depth.ts", "nir", &out, &errs); code == 0 || out.Len() != 0 || !strings.Contains(errs.String(), "budget") {
		t.Fatalf("depth result %d %s", code, &errs)
	}
}
