# gopdftotext

Simple service that converts PDF document to plaintext. gopdftotext accepts PDF
files over HTTP POST and returns page response in plaintext.

Initial implementation is a simple wrapper around
[Poppler](https://poppler.freedesktop.org/) but might change in the future. It
comes with a Docker file for dependency management so it's advised that you
build docker image if popler is not available locally.

This service can be used as a building block in ETL pipelines (RAG :rocket:).

## Example

Post file to the service

```sh
curl -X POST -F file=@input-file.pdf localhost:8888 | jq
```

## Build

```sh
GOOS=linux CGO_ENABLED=0 go build
docker build -t gopdftotext .
```
