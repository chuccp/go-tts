# Text frontend data

## Chinese pinyin

These files are copied unchanged from `pypinyin` 0.55.0:

- `pinyin_dict.json`
- `phrases_dict.json`

Source: <https://github.com/mozillazg/python-pinyin>

License: MIT. The phrase dictionary is used for longest-word matching so
polyphonic Chinese characters use their context-specific pronunciation.

SHA-256:

```text
5f294c01e6c6c0a1c8e329c79335a3f8e0b27d06bf1de7a99244b765892d1e5b  pinyin_dict.json
a45ff140a6b631ca9c82127b280a2f414e0aba6bb2824a0e9d1e77fff359c665  phrases_dict.json
```

## Japanese pronunciations

`ipa_pronunciations.tsv.gz` is a gzip-compressed `token ID -> pronunciation`
table generated from `github.com/ikawaha/kagome-dict/ipa` v1.2.6. The compact
table lets the runtime use Kagome's smaller dictionary representation while
preserving the tokenizer IDs, context segmentation and Kanji readings.

Regenerate it with:

```powershell
go run scripts/export-kagome-pronunciations.go
```

Source: <https://github.com/ikawaha/kagome-dict>

License: Apache-2.0.

```text
ad0082d55055e68d1ee81e70685f7c465bd0ecd7a38f54f0295ca9620d1d776d  ipa_pronunciations.tsv.gz
```
