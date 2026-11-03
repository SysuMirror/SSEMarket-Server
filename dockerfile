FROM golang

WORKDIR "/app"

# Debian换源
RUN sed -i 's/deb.debian.org/mirrors.aliyun.com/g' /etc/apt/sources.list.d/debian.sources
# 安装MySQL客户端   实际上只是为了使用mysqldump备份；ffmpeg/gifsicle 用于动图保帧压缩
# （apt 层放在 COPY 之前：改代码重新 build 时不用重新下载系统包）
RUN apt-get update && apt-get install -y default-mysql-client ffmpeg gifsicle
ENV GOPROXY=https://goproxy.cn,direct

COPY . .
RUN go mod download
RUN go build -o serve

CMD [ "./serve" ]
