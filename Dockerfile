FROM ubuntu:latest

RUN apt-get update &&\
    apt-get install wget git gcc protobuf-compiler libglfw3 libglfw3-dev -y

RUN mkdir /go &&\
    wget https://redirector.gvt1.com/edgedl/go/go1.8.3.linux-amd64.tar.gz &&\
    tar -C /usr/local -xzf go1.8.3.linux-amd64.tar.gz &&\
    rm /go1.8.3.linux-amd64.tar.gz

ENV PATH=$PATH:/usr/local/go/bin
ENV GOPATH=/go
ENV GOBIN=$GOPATH/bin
ENV PATH=$PATH:$GOBIN

RUN go get -u github.com/golang/protobuf/protoc-gen-go &&\
    go get -u github.com/whyrusleeping/gx &&\
    go get -u github.com/whyrusleeping/gx-go &&\
    go get github.com/q6r/umbra; exit 0

RUN cd /go/src/github.com/q6r/umbra &&\
    gx --verbose install &&\
    protoc --go_out=. core/payload/*.proto &&\
    cd ./core &&\
    go get -v &&\
    cd ../nk &&\
    go get -v &&\
    go build
