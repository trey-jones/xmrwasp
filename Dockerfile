FROM golang:1.9-alpine
MAINTAINER Trey Jones "trey@eyesoreinc.com"

COPY ./ $GOPATH/src/github.com/trey-jones/xmrwasp/

WORKDIR $GOPATH/src/github.com/trey-jones/xmrwasp

RUN go install

WORKDIR /config
RUN touch config.json
VOLUME /config

ENTRYPOINT ["xmrwasp"]
