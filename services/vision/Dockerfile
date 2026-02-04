FROM python:3.11-slim

# Instala dependências de sistema para OpenCV
RUN apt-get update && apt-get install -y \
	libgl1-mesa-glx \
	libglib2.0-0 \
	libsm6 \
	libxext6 \
	libxrender-dev \
	libgomp1 \
	&& rm -rf /var/lib/apt/lists/*

WORKDIR /app

# Copia requirements
COPY requirements.txt .

# Instala dependências Python
RUN pip install --no-cache-dir -r requirements.txt

# Copia código fonte
COPY src/ ./src/

# Variáveis de ambiente
ENV NATS_URL=nats://nats:4222
ENV PYTHONUNBUFFERED=1

# Comando para iniciar o solver
CMD ["python", "-m", "src.captcha_solver"]
