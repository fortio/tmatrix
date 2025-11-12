FROM scratch
COPY NAME /usr/bin/NAME
ENV HOME=/home/user
ENTRYPOINT ["/usr/bin/NAME"]
