FROM alpine:3.23

RUN ln -sf /usr/share/zoneinfo/Asia/Shanghai /etc/localtime \
    && echo 'LANG="en_US.UTF-8"' > /etc/locale.conf
ENV LANG=en_US.UTF-8 \
    LANGUAGE=en_US.UTF-8

RUN apk add --no-cache curl net-tools
RUN mkdir -p /data/accelerboat/logs && mkdir -p /data/workspace
ADD --chmod=755 accelerboat /data/workspace/

WORKDIR /data/accelerboat/
CMD [ "/data/workspace/accelerboat" ]
