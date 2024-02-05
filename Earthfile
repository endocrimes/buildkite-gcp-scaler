VERSION 0.7
FROM us.gcr.io/bluecore-ops/dockerfiles/golang:lint-1.17
WORKDIR /app
ENV GOPRIVATE=github.com/TriggerMail

ci:
  BUILD +tests
  BUILD +docker
  BUILD +lint

go-mod:
    RUN git config --global url."git@github.com:".insteadOf "https://github.com/"
    RUN mkdir -p -m 0600 ~/.ssh && ssh-keyscan github.com >> ~/.ssh/known_hosts
    COPY go.mod go.sum .
    RUN --ssh go mod download

vendor:
    FROM +go-mod
    COPY --dir pkg scaler main.go /app
    RUN --ssh go mod vendor
    SAVE ARTIFACT . /files

tests:
    FROM +vendor
    RUN go test -v ./... -cover

lint:
    FROM +vendor
    RUN go fmt ./...

build:
    FROM +vendor
    RUN go build -mod=vendor -o bin/buildkite-gcp-autoscaler main.go
    SAVE ARTIFACT ./bin/buildkite-gcp-autoscaler /buildkite-gcp-autoscaler

docker:
    FROM us.gcr.io/bluecore-ops/dockerfiles/golang:lint-1.17
    ARG EARTHLY_GIT_SHORT_HASH
    COPY +build/buildkite-gcp-autoscaler /buildkite-gcp-autoscaler
    ENTRYPOINT ["/buildkite-gcp-autoscaler"]
    SAVE IMAGE --push us.gcr.io/bluecore-ops/apps/buildkite-gcp-autoscaler:${EARTHLY_GIT_SHORT_HASH}
