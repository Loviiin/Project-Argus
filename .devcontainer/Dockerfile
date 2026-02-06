FROM mcr.microsoft.com/devcontainers/base:ubuntu-22.04


RUN curl -L https://github.com/yt-dlp/yt-dlp/releases/latest/download/yt-dlp -o /usr/local/bin/yt-dlp \
	&& chmod a+rx /usr/local/bin/yt-dlp

RUN apt-get update && export DEBIAN_FRONTEND=noninteractive \
	&& apt-get -y install --no-install-recommends \
	ffmpeg \
	libgl1 \
	libglib2.0-0 \
	libsm6 \
	libxext6 \
	&& apt-get clean -y && rm -rf /var/lib/apt/lists/*
