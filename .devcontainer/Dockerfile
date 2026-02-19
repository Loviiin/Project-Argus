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
	libnss3 \
	libatk1.0-0 \
	libatk-bridge2.0-0 \
	libcups2 \
	libdrm2 \
	libgtk-3-0 \
	libgbm1 \
	libasound2 \
	libpangocairo-1.0-0 \
	libxshmfence1 \
	libxcomposite1 \
	libxdamage1 \
	libxrandr2 \
	libxi6 \
	libxtst6 \
	&& apt-get clean -y && rm -rf /var/lib/apt/lists/*

# Display virtual + noVNC para acesso via browser
RUN apt-get update && export DEBIAN_FRONTEND=noninteractive \
	&& apt-get -y install --no-install-recommends \
	xvfb \
	x11vnc \
	novnc \
	websockify \
	&& apt-get clean -y && rm -rf /var/lib/apt/lists/*

# Script de inicialização do display virtual
COPY start-vnc.sh /usr/local/bin/start-vnc.sh
RUN chmod +x /usr/local/bin/start-vnc.sh
