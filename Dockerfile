FROM golang:1.9-alpine
MAINTAINER Trey Jones "trey@eyesoreinc.com"

COPY ./ $GOPATH/src/github.com/trey-jones/xmrwasp/
# go get github.com/trey-jones/xmrwasp

WORKDIR $GOPATH/src/github.com/trey-jones/xmrwasp

RUN go install

WORKDIR /config
VOLUME /config

ENTRYPOINT ["xmrwasp"]
