# Build the manager binary
FROM golang:1.16 as builder
WORKDIR /workspace

# Copy the Go Modules manifests & cache them before building and copying source code, so when source code changes,
# the downloaded dependencies stay cached and won't need to be downloaded again (unless go.mod/go.sum get invalidated)
COPY go.mod go.mod
COPY go.sum go.sum
RUN go mod download

# Copy the go source
COPY *.go api controllers internal ./
RUN ls -la
RUN fail-now

# Build
ENV CGO_ENABLED="0"
ENV GOARCH="amd64"
ENV GOOS="linux"
ENV GO111MODULE="on"
RUN make build

# Use distroless as minimal base image to package the manager binary
# Refer to https://github.com/GoogleContainerTools/distroless for more details
FROM gcr.io/distroless/static:nonroot
WORKDIR /
COPY --from=builder /workspace/bin/manager ./manager
USER 65532:65532

ENTRYPOINT ["/manager"]
