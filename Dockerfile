# Builder: compile static binary (CGO disabled).
FROM golang:1.26.1-alpine AS builder

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .

ARG VERSION=dev
ARG COMMIT=unknown
ARG DATE=unknown

ENV CGO_ENABLED=0

RUN go build \
	-ldflags "-X github.com/mab-go/xmind-mcp/internal/version.Version=${VERSION} -X github.com/mab-go/xmind-mcp/internal/version.Commit=${COMMIT} -X github.com/mab-go/xmind-mcp/internal/version.Date=${DATE}" \
	-o /xmind-mcp \
	./cmd/xmind-mcp

# Runtime: minimal image with only the binary (stdio MCP).
FROM gcr.io/distroless/static-debian12

LABEL io.modelcontextprotocol.server.name="io.github.mab-go/xmind-mcp"

COPY --from=builder /xmind-mcp /xmind-mcp

ENTRYPOINT ["/xmind-mcp"]
