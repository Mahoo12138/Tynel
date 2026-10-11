module tynel.local/tsgo-host

go 1.26.0

toolchain go1.26.7

require github.com/microsoft/typescript-go v0.0.0

require (
	github.com/go-json-experiment/json v0.0.0-20260623181947-01eb4420fa68 // indirect
	github.com/klauspost/cpuid/v2 v2.2.10 // indirect
	github.com/zeebo/xxh3 v1.1.0 // indirect
	golang.org/x/sync v0.21.0 // indirect
	golang.org/x/text v0.38.0 // indirect
)

replace github.com/microsoft/typescript-go => ../../.upstream/tsgo-v7.0.2/TypeScript-1e4744d68260a7cb91b62b12edc3f6a2187faaf1/tsc
