/**
 * xbogus.js — TikTok X-Bogus Parameter Generator
 *
 * Implementação pura em JavaScript (sem browser/puppeteer).
 * Reverse-engineered da lógica client-side do TikTok.
 *
 * O X-Bogus é um parâmetro anti-bot que codifica:
 *   - Hash dos parâmetros da query
 *   - Hash do User-Agent
 *   - Timestamp
 *   - Fingerprint do "canvas" (constante estática)
 *
 * Resultado: string de ~28 caracteres usando um charset base64 customizado.
 */

const crypto = require('crypto');

// Charset customizado do TikTok (64 caracteres + padding '=')
const CHARS = 'Dkdpgh4ZKsQB80/Mfvw36XI1R25-WUAlEi7NLboqYTOPuzmFjJnryx9HVGcaStCe=';

// ── Helpers ──────────────────────────────────────────────────────

function md5Bytes(input) {
  return [...crypto.createHash('md5').update(input, 'utf-8').digest()];
}

/** Codifica 3 bytes → 4 caracteres usando o charset do TikTok. */
function encodeTriple(a, b, c) {
  return (
    CHARS[(a >> 2) & 0x3f] +
    CHARS[((a & 0x03) << 4) | ((b >> 4) & 0x0f)] +
    CHARS[((b & 0x0f) << 2) | ((c >> 6) & 0x03)] +
    CHARS[c & 0x3f]
  );
}

/** Codifica um array de bytes inteiro para a string X-Bogus. */
function encodeBytes(bytes) {
  let out = '';
  for (let i = 0; i < bytes.length; i += 3) {
    out += encodeTriple(bytes[i] || 0, bytes[i + 1] || 0, bytes[i + 2] || 0);
  }
  return out;
}

// ── Gerador principal ────────────────────────────────────────────

/**
 * Gera o parâmetro X-Bogus para uma query string + User-Agent.
 *
 * @param {string} queryString  Parâmetros da URL (sem o '?' inicial)
 * @param {string} userAgent    User-Agent do "browser"
 * @returns {string}            Valor do X-Bogus (~28 chars)
 */
function generateXBogus(queryString, userAgent) {
  const ts = Math.floor(Date.now() / 1000);

  // 1. Hashes MD5
  const paramsHash = md5Bytes(queryString);
  const uaHash     = md5Bytes(userAgent);
  const canvasHash = md5Bytes('canvas_fp_v2');   // fingerprint estático

  // 2. Payload de 19 bytes
  const payload = new Uint8Array(19);

  // Versão do algoritmo
  payload[0] = 0x02;
  payload[1] = 0x01;

  // Canvas fingerprint (2 bytes selecionados)
  payload[2] = canvasHash[14];
  payload[3] = canvasHash[15];

  // Timestamp big-endian 32-bit
  payload[4] = (ts >>> 24) & 0xff;
  payload[5] = (ts >>> 16) & 0xff;
  payload[6] = (ts >>> 8)  & 0xff;
  payload[7] =  ts         & 0xff;

  // Params hash (bytes selecionados do MD5)
  payload[8]  = paramsHash[0];
  payload[9]  = paramsHash[1];
  payload[10] = paramsHash[14];
  payload[11] = paramsHash[15];

  // UA hash (bytes selecionados do MD5)
  payload[12] = uaHash[0];
  payload[13] = uaHash[1];
  payload[14] = uaHash[14];
  payload[15] = uaHash[15];

  // Plataforma
  payload[16] = 0x01;  // web_pc
  payload[17] = 0x08;  // sub-platform

  // Checksum XOR de todos os bytes anteriores
  let checksum = 0;
  for (let i = 0; i < 18; i++) checksum ^= payload[i];
  payload[18] = checksum;

  // 3. XOR cipher com chave derivada do UA hash
  const ciphered = new Uint8Array(payload.length);
  for (let i = 0; i < payload.length; i++) {
    ciphered[i] = payload[i] ^ uaHash[i % uaHash.length];
  }

  // 4. Encode para string final
  return encodeBytes([...ciphered]);
}

/**
 * Gera um msToken aleatório (simulação).
 * O TikTok tipicamente emite um de ~120+ chars via cookie.
 *
 * @param {number} [length=126]
 * @returns {string}
 */
function generateMsToken(length = 126) {
  const chars = 'ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-_';
  const bytes = crypto.randomBytes(length);
  let token = '';
  for (let i = 0; i < length; i++) {
    token += chars[bytes[i] % chars.length];
  }
  return token;
}

module.exports = { generateXBogus, generateMsToken };
