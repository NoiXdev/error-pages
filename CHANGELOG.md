## 1.0.0-rc.1 (2026-11-10)


### Features

* add locales and text catalog ([a803556](https://github.com/NoiXdev/error-pages/commit/a803556736365ebc4ee03fa5d590a742ffe29229))
* add module scaffold and page registry ([396fdba](https://github.com/NoiXdev/error-pages/commit/396fdba6869b6e38f15b9df393df6017c96cba75))
* **cmd:** add CLI with serve, healthcheck, maintenance and version ([daa5fa9](https://github.com/NoiXdev/error-pages/commit/daa5fa9f58bc8366b56739ecacb7720d48ad02cb))
* **config:** load and validate configuration from file and env ([6e179e6](https://github.com/NoiXdev/error-pages/commit/6e179e675bab58ea1502eb5d8fd318c60064bfed))
* **lang:** add Accept-Language negotiation ([6a73b05](https://github.com/NoiXdev/error-pages/commit/6a73b0559d921cee8a965764720105dfdb0783b2))
* **maintenance:** add maintenance mode state and switch ([d9b5121](https://github.com/NoiXdev/error-pages/commit/d9b512174cd11e8a846d8967a9546ec9d6ee2cc0))
* pick a random theme per request ([f40d5b8](https://github.com/NoiXdev/error-pages/commit/f40d5b8fb8a3bffa2cc9b02f4850a88f59da9fb5))
* **render:** add renderer, shared layout and glass theme ([ec03757](https://github.com/NoiXdev/error-pages/commit/ec03757e1c0425dee5c8032a87365841a4c3b699))
* **server:** add HTTP handler for pages, health check and static assets ([442246a](https://github.com/NoiXdev/error-pages/commit/442246a280d4154e406c487a04bf58c94bf95df4))
* **themes:** add classic and clean themes ([20b19ff](https://github.com/NoiXdev/error-pages/commit/20b19ffa4548326beaf564f7d82a5addab615f56))
* **themes:** add status theme ([a8d931a](https://github.com/NoiXdev/error-pages/commit/a8d931aa89dd63c8494cdbcd14cf04918f05a68d))
* **themes:** add terminal theme ([7cbae23](https://github.com/NoiXdev/error-pages/commit/7cbae230f926376fe45467b624074342a7c53ca0))


### Bug Fixes

* **build:** cross-compile for the target platform ([778a45d](https://github.com/NoiXdev/error-pages/commit/778a45deba083de75c631b1700938540392b3190))
* **cli:** log start after listening and make serve testable ([e68ae35](https://github.com/NoiXdev/error-pages/commit/e68ae350d20e1100333ad2eb32d14cc24cb9f364))
* **cli:** set an explicit idle timeout ([2aa69a3](https://github.com/NoiXdev/error-pages/commit/2aa69a395b438a38dab172255a3765582595058c))
* **config:** harden link URL validation and reject unknown YAML keys ([350a593](https://github.com/NoiXdev/error-pages/commit/350a5939ca154e35f4367e21c8cf3d79fb1aaea1))
* **config:** reject duplicate link ids ([6075abd](https://github.com/NoiXdev/error-pages/commit/6075abd8fdb102beb9827e92c7634bad82100d02))
* **i18n:** ignore stored languages that are not own keys ([568d842](https://github.com/NoiXdev/error-pages/commit/568d842883924f2e51355bfa3202114c14c2849a))
* **render:** inline assets with CSP hashes ([5140d75](https://github.com/NoiXdev/error-pages/commit/5140d75e62b811db22e0c96ee10789b34eaf5e16))
* **render:** keep language texts consistent across switches ([8592d48](https://github.com/NoiXdev/error-pages/commit/8592d48f974802ae793db995e27aa0174a5b3eea))
* **server:** let page paths win over X-Code and negotiate Accept by q ([3d5598b](https://github.com/NoiXdev/error-pages/commit/3d5598b4223fea9eb61d36a3cb6bcdd7ef8e80c1))
* **themes:** align clean, status and terminal with the mockup ([fea69bb](https://github.com/NoiXdev/error-pages/commit/fea69bb22e24f4e091d17af4f5381113b3a3e871))
* **themes:** raise muted-text contrast and tidy scripts ([10bcf93](https://github.com/NoiXdev/error-pages/commit/10bcf93a816f16bae6aa1384c590857774c76200)), closes [#777](https://github.com/NoiXdev/error-pages/issues/777) [#6b6b6](https://github.com/NoiXdev/error-pages/issues/6b6b6) [#8b86a3](https://github.com/NoiXdev/error-pages/issues/8b86a3) [#6f6a88](https://github.com/NoiXdev/error-pages/issues/6f6a88)
* tighten inline guard, settle background tabs, drop status mono font ([30fc6e8](https://github.com/NoiXdev/error-pages/commit/30fc6e8f1c7ac9a2e99c9b646b9871ade423564e))

