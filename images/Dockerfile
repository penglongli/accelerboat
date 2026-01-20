FROM centos:7

RUN ln -sf /usr/share/zoneinfo/Asia/Shanghai /etc/localtime \
    && echo 'LANG="en_US.UTF-8"' > /etc/locale.conf
ENV LANG=en_US.UTF-8 \
    LANGUAGE=en_US.UTF-8

RUN mkdir -p /data/accelerboat/logs
ADD accelerboat /data/accelerboat/
RUN chmod +x /data/accelerboat/accelerboat

WORKDIR /data/accelerboat/
CMD [ "/data/accelerboat/accelerboat" ]
