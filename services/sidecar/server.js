/**
 * server.js — Sidecar de Assinatura TikTok
 *
 * Contrato:
 *   POST /sign   { url, user_agent }  →  { signed_url, headers, error }
 *   GET  /health                      →  { status: "ok" }
 *
 * Uso:
 *   npm start          (produção)
 *   npm run dev         (auto-reload com --watch)
 */

const express = require('express');
const { generateXBogus, generateMsToken } = require('./xbogus');

const app = express();
app.use(express.json());

const PORT = process.env.PORT || 8000;
const DEFAULT_UA =
  'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 ' +
  '(KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36';

// ── POST /sign ───────────────────────────────────────────────────
app.post('/sign', (req, res) => {
  const { url: targetUrl, user_agent: ua } = req.body || {};

  if (!targetUrl) {
    return res.status(400).json({ signed_url: '', headers: {}, error: 'url is required' });
  }

  try {
    const userAgent = ua || DEFAULT_UA;
    const parsedUrl = new URL(targetUrl);
    const queryString = parsedUrl.search.slice(1); // remove '?'

    // 1. Gerar X-Bogus
    const xBogus = generateXBogus(queryString, userAgent);
    parsedUrl.searchParams.set('X-Bogus', xBogus);

    // 2. Gerar msToken
    const msToken = generateMsToken();
    parsedUrl.searchParams.set('msToken', msToken);

    const signedUrl = parsedUrl.toString();

    console.log(`[sidecar] ✅ Signed (XB=${xBogus.slice(0, 12)}…) → ${signedUrl.slice(0, 90)}…`);

    return res.json({
      signed_url: signedUrl,
      headers: {
        'User-Agent': userAgent,
        Cookie: `msToken=${msToken}; tt_webid=7000000000000000000`,
      },
      error: '',
    });
  } catch (err) {
    console.error(`[sidecar] ❌ Erro: ${err.message}`);
    return res.status(500).json({ signed_url: '', headers: {}, error: err.message });
  }
});

// ── GET /health ──────────────────────────────────────────────────
app.get('/health', (_req, res) => {
  res.json({ status: 'ok' });
});

// ── Start ────────────────────────────────────────────────────────
app.listen(PORT, () => {
  console.log(`🚀 Argus Sidecar rodando em http://localhost:${PORT}`);
  console.log(`   POST /sign   — assina URLs com X-Bogus + msToken`);
  console.log(`   GET  /health — health check`);
});
