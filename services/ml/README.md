# ML — Captcha Training Data

Estrutura de coleta e treinamento para o modelo de resolução automática de captchas.

## Estrutura

```
ml/
├── data/
│   ├── raw/            # Amostras coletadas automaticamente durante execução
│   │   ├── rotate/     # Pares outer+inner (captcha de rotação)
│   │   └── slider/     # Pares background+piece (captcha de slider)
│   └── labeled/        # Amostras com label confirmada (após análise manual)
│       ├── rotate/     # {angle: float} confirmado
│       └── slider/     # {offset_x: float} confirmado
├── models/             # Checkpoints dos modelos treinados
└── scripts/
    ├── collect.py      # Utilitário para mover raw → labeled com anotação
    └── train.py        # Script de treinamento (a definir com tech lead)
```

## Pipeline de coleta

Cada vez que o Discovery encontra um captcha e o fallback manual é acionado:

1. O Go salva automaticamente as imagens em `data/raw/<tipo>/<timestamp>_*.png`
2. Um `<timestamp>_meta.json` é gerado com o tipo e horário
3. Você resolve o captcha manualmente
4. Depois, você roda `scripts/collect.py` para anotar o sample (adicionar o ângulo/offset correto) e mover para `data/labeled/`

## Formato dos metadados (`_meta.json`)

```json
{
  "type": "rotate",
  "timestamp": 1708354800000,
  "manual": true,
  "image_keys": ["outer", "inner"]
}
```

## Próximos passos (a definir com tech lead)

- [ ] Definir arquitetura do modelo (CNN, ResNet, ViT, etc.)
- [ ] Definir formato da label (ângulo em graus, offset em pixels, normalizado?)
- [ ] Implementar `scripts/collect.py` (interface de anotação)
- [ ] Implementar `scripts/train.py`
- [ ] Definir threshold de confiança para substituir resolução automática atual
