FROM scratch
COPY tmatrix /usr/bin/tmatrix
ENV HOME=/home/user
ENTRYPOINT ["/usr/bin/tmatrix"]
