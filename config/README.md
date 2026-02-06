# Config Directory

Contains application configuration files.

## Files

- `config.yaml` - **Active config (gitignored)**
- `config.example.yaml` - Template with all options documented

## Setup

```bash
cp config/config.example.yaml config/config.yaml
# Edit config.yaml with your settings
```

## Important Sections

### Premium Features

The `captcha` section controls advanced anti-detection features:

```yaml
captcha:
  humanized_movement:
    enabled: true # Toggle premium features
```

**When enabled: true**

- Uses Bézier curves, overshoot, tremor
- Gaussian delay distribution
- Success rate: ~80-85%

**When enabled: false**

- Basic linear movement
- Fixed delays
- Success rate: ~30-40%
- **Can be open sourced**

## Security

**DO NOT COMMIT**:

- `config.yaml` (contains your settings)
- Any files with real credentials

**SAFE TO COMMIT**:

- `config.example.yaml` (template with all premium features documented)

## For Open Source Release

When creating public version, remove from `config.example.yaml`:

- All `captcha.humanized_movement` specific values
- All `captcha.delays` numbers
- All `captcha.anti_detection` section

Replace with basic config only.

```

Environment variables can override settings. Parser enables `viper.AutomaticEnv()` with `.` replaced by `_`, so `DATABASE_URL` overrides `database.url`.
```
