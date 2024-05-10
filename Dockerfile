FROM alpine

ARG TARGETOS TARGETARCH

WORKDIR /app

COPY bin/${TARGETOS}_${TARGETARCH}/controller .

CMD ["./controller"]
