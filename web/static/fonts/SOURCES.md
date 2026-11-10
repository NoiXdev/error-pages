# Font sources

Fetched once with `npm pack` (no install, no package scripts) from the npm
registry. Only the latin and latin-ext `wght-normal` WOFF2 files are kept.
Both fonts are licensed under the SIL Open Font License 1.1 (see `OFL.txt`).

| Package | Version | Tarball | Tarball sha256 |
|---|---|---|---|
| `@fontsource-variable/inter` | 5.3.0 | https://registry.npmjs.org/@fontsource-variable/inter/-/inter-5.3.0.tgz | `d26710dd38e7217484a1d47ee76c024977ec550548876f26d06ed1844af09a14` |
| `@fontsource-variable/inconsolata` | 5.3.0 | https://registry.npmjs.org/@fontsource-variable/inconsolata/-/inconsolata-5.3.0.tgz | `274b0bbc06ef8c74982eca67d4206ad0984767e9d72957e87724bedd0e405aea` |

| File | Package path | sha256 |
|---|---|---|
| `inter-latin-wght-normal.woff2` | `@fontsource-variable/inter@5.3.0/files/inter-latin-wght-normal.woff2` | `3100e775e8616cd2611beecfa23a4263d7037586789b43f035236a2e6fbd4c62` |
| `inter-latin-ext-wght-normal.woff2` | `@fontsource-variable/inter@5.3.0/files/inter-latin-ext-wght-normal.woff2` | `34b9c504cab7a73e37b746343a449132e56cf7b5481af2cb81dc74dcff25c956` |
| `inconsolata-latin-wght-normal.woff2` | `@fontsource-variable/inconsolata@5.3.0/files/inconsolata-latin-wght-normal.woff2` | `4914e68b98699a8f3d5d552fe60fd7a1c2cb631a0f66f0a148392f161d3155ab` |
| `inconsolata-latin-ext-wght-normal.woff2` | `@fontsource-variable/inconsolata@5.3.0/files/inconsolata-latin-ext-wght-normal.woff2` | `8ca8881c29e485da4cfd38974399d070fe8fe934d626f774a32a59898117a676` |

The `unicode-range` values in `web/static/css/base.css` are copied from the
packages' `wght.css`.
