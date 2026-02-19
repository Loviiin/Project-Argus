#!/bin/bash
# start-vnc.sh - Sobe display virtual Xvfb + x11vnc + noVNC
# Acesse em: http://localhost:6080

set -e

# Mata instâncias anteriores silenciosamente
pkill -f "Xvfb :99" 2>/dev/null || true
pkill -f "x11vnc"   2>/dev/null || true
pkill -f "websockify" 2>/dev/null || true
sleep 1

# Inicia display virtual (1280x800, 24-bit color)
Xvfb :99 -screen 0 1280x800x24 &
export DISPLAY=:99
echo "🖥️  Display virtual :99 iniciado (1280x800)"

# Aguarda Xvfb estar pronto
sleep 1

# Inicia servidor VNC sem senha (só localhost)
x11vnc -display :99 -forever -nopw -shared -quiet &
echo "🔗 VNC server rodando na porta 5900"

# Inicia noVNC (web interface via websocket)
websockify --web /usr/share/novnc 6080 localhost:5900 &

# Aguarda o websockify estar pronto antes de sair
sleep 2
echo "✅ noVNC iniciado em http://localhost:6080"
echo "   Abra no seu browser e clique em 'Connect'"
