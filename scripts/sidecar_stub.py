"""
Sidecar Stub para testes locais.
Simula o serviço de assinatura de URLs do TikTok (POST /sign).
Retorna a URL original sem assinatura real — apenas para validar o fluxo E2E.

Uso:
    pip install flask
    python sidecar_stub.py

O serviço fica disponível em http://localhost:8000/sign
"""

from flask import Flask, request, jsonify

app = Flask(__name__)


@app.route("/sign", methods=["POST"])
def sign_url():
    data = request.get_json(force=True)
    url = data.get("url", "")
    user_agent = data.get("user_agent", "")

    if not url:
        return jsonify({"signed_url": "", "error": "url is required"}), 400

    # Stub: retorna a URL original com um parâmetro fake X-Bogus
    signed = f"{url}&X-Bogus=STUB_SIGNATURE_FOR_TESTING"

    print(f"[sidecar-stub] Signed URL for UA={user_agent[:30]}...")
    print(f"[sidecar-stub] → {signed[:100]}...")

    return jsonify({
        "signed_url": signed,
        "headers": {
            "Cookie": "msToken=stub_token_for_testing; tt_webid=stub_webid"
        },
        "error": ""
    })


@app.route("/health", methods=["GET"])
def health():
    return jsonify({"status": "ok"})


if __name__ == "__main__":
    print("🚀 Sidecar Stub rodando em http://localhost:8000")
    print("   POST /sign  — assina URLs (stub)")
    print("   GET  /health — health check")
    app.run(host="0.0.0.0", port=8000, debug=False)
