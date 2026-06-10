# Fleet — Analyse des lacunes & roadmap

> Audit gap multi-agent (77 findings récoltés → 76 confirmés / 1 rejeté par passe adversariale). 2026-06-09.

## 1. Verdict global

Fleet est **structurellement complet mais opérationnellement fragile** : les deux specs (v1.0 4-axes, v1.1 wizard) sont implémentées dans leurs grandes lignes, mais le cœur de la proposition de valeur est cassé et la couche d'erreurs est quasi inexistante. **La plus grosse lacune unique : l'Axe 4 (token-saving) est totalement neutralisé** — chaque agent reçoit un `/relay talk` automatique au boot (`runner.go:169-172`), parce que `IsExecutive` n'est jamais mis à `true` et que `AutoTalk` (le vrai knob spec'é) est du code mort. En second rideau, une culture systémique du *silent error swallowing* fait qu'un lancement partiel, un relay tombé ou un tmux cassé sont tous rapportés comme des succès.

## 2. Top lacunes par thème

### Features manquantes (le plus critique)
- **Axe 4 défait** (`runner.go:169-172`, `config.go:66`) : le `/relay talk` auto au boot annule l'économie de tokens. `AutoTalk` est parsé, tagué TOML, round-trip-testé mais **lu par zéro ligne de prod** ; le gating utilise `!IsExecutive`, un champ jamais mis à `true` par aucun preset/wizard.
- **`fleet stop` laisse un agent fantôme sur le relay** (`main.go:331-367`) : aucun `deactivate_agent`/`delete_agent` (méthode absente du client, `client.go` s'arrête à 196). `list_agents`, la liste du wizard et le routing accumulent des entrées mortes.
- **`fleet add` crée un agent à moitié cassé** (`main.go:285-329`) : pas de relay register, pas de `profile_slug`, pas de rename/color/talk, pas de `Validate()`. L'agent apparaît dans la grille mais **ne voit aucune tâche dispatchée**.
- **Pas d'observabilité relay** (`main.go:147-181`) : `--status` devine busy/idle en cherchant le glyphe `❯` dans le pane tmux (`tmux.go:74-80`). Les endpoints relay (`list_tasks`, `get_inbox`, `query_context`) ne sont **jamais** interrogés.
- **`--relay-url` / `--force` jamais implémentés** (`main.go:35-39`).

### Robustesse / Errors (culture du silence)
- **`launch()` jette toutes les erreurs par agent** (`main.go:449-467`) : ne compte que `r.Success`, checkmark vert même sur échec partiel, exit 0. Sessions orphelines jamais rollback (`runner.go:45-66`).
- **`ConfigureAgentsAsync` fire-and-forget** (`runner.go:81,182,191`) : MkdirAll/WriteFile/cmd.Start, toutes erreurs jetées, signature `void`. Config muette qui ne tourne pas mais affiche « Agents configuring in background ».
- **tmux cassé = « no sessions »** (`tmux.go:85-88,106-109`) : `(nil,nil)` sur toute erreur exec → les kill ne font rien silencieusement.
- **`--kill` escalade en `--kill-all`** (`main.go:110-114`) : toute erreur de `LoadLast()` tue **tous les fleets de tous les projets** sans confirmation.
- **Relay JSON-RPC errors avalées** (`client.go:40-100`) : `mpcResponse` sans champ `Error`, pas de check `StatusCode`/`isError`. `Health()` peut faussement réussir.
- **`runStop` : sleep 3s aveugle puis force-kill** (`main.go:344-358`) : ignore les prompts `/exit`, les erreurs `KillSession`/`SaveAsLast`.

### Sécurité
- **Command injection via nom de projet** (`runner.go:134-135`, `config.go:77`, `project_panel.go:261`) : basename brut, jamais validé, interpolé non-quoté dans le bash généré. Dossier nommé `$(...)` = RCE au lancement.
- **Role non contraint → RCE** (`runner.go:121-123`) : texte libre interpolé dans le bash, escaping qui rate le backslash.
- **AppleScript injection** (`iterm.go:64,122,132`) : `SessionName` concaténée dans des littéraux AppleScript lancés via `osascript`.
- **Path traversal** (`config.go:129,134-136`) : `Project.Name` dans `filepath.Join` + symlink, sans validation → écriture arbitraire via `../`.
- **Vault uploadé en clair** (`runner.go:160-166`, `vault.go:46-59`) : `.md` envoyé verbatim au relay via `set_memory`, HTTP par défaut, sans filtrage de secrets.

### Tests
- **`cmd/fleet` : 0%** ; `runner` 5% ; `doctor` 19% ; `wizard` 1.5%.
- **Aucun seam de command-runner** (`tmux.go:29-154`, `doctor.go`) : tout `exec.Command` direct.
- **`ConfigureAgentsAsync` : 100+ lignes de bash hand-escaped, 0 test** (`runner.go:79-192`).
- **`normalizeName` non testé** (`util.go:22-33`) : ne strip pas les tirets en tête → `-lead` passe le wizard puis échoue `Validate()`.

### UX
- **Erreurs avalées dans le wizard** (`wizard_model.go:76,119,149`, `project_panel.go:265`) : relay down / scan échoué / MkdirAll échoué → liste vide ou projet faussement « ready », zéro feedback.
- **`fleet logs -f` efface tout l'écran** (`main.go:273`) via `\033[2J\033[H`, sans header/hint Ctrl-C, `return nil` sur capture error.
- **Wizard ne s'adapte pas au resize** (`wizard_model.go:49-60`) : largeurs hardcodées, `rightWidth` peut devenir négatif.
- **Pas de toggle `IsExecutive` dans le wizard** (`agent_drawer.go:228-233`) : aucun moyen de désigner le boss, edit drop silencieusement `IsExecutive`/`AutoTalk`.

### Portabilité
- **Aucun `runtime.GOOS`** (`doctor.go:35,76,80`, `iterm.go:30,42`) : `brew install` et check iTerm2 affichés même sur Linux.
- **`bash` hardcodé** (`runner.go:94,189`) sans `LookPath`.
- **Glyphe `❯` (U+276F) hardcodé** (`tmux.go:66,79`, `runner.go:99`) comme signal de readiness : changement thème/locale → timeout `wait_prompt` 90s.
- **`iterm.go` utilise `os.Getenv("HOME")`** au lieu de `config.FleetDir()`.

### Architecture
- **Protocole relay réimplémenté 2×** (`runner.go:108-167`) : JSON-RPC en strings curl ET dans le `relay.Client` typé. `EnsureProfile`/`PushVaultDoc`/`ListProfiles` morts.
- **2 parsers divergents** pour `fleet-{project}-{agent}` (`main.go:187` vs `tmux.go:99`) : `extractProject` splitte sur le dernier tiret → `ux-designer` casse le grouping `--status`.
- **Palette dupliquée 5×** sur 3 packages (`config.go:71`).
- **Default relay URL en double** (`main.go:18` vs `runner.go:90`).

## 3. Roadmap priorisée

| # | Item | Thème | Effort | Impact |
|---|------|-------|--------|--------|
| **P0** | Implémenter Axe 4 : retirer `/relay talk` auto (`runner.go:169-172`), gater sur `AutoTalk` au lieu de `!IsExecutive` (`runner.go:169,208,223`), s'appuyer sur dispatch→WakeAgent | features | S | **Critique** — restaure l'économie de tokens, raison d'être du produit |
| **P0** | Valider `Project.Name` et `Role` (regex `validName`) dans `Validate()` + normaliser le basename wizard (`config.go:77`, `project_panel.go:261`, `runner.go:121`) | sécurité | S | **Critique** — ferme la RCE (bash + AppleScript) et le path traversal |
| **P0** | `--kill` : ne plus fall-through vers `--kill-all` sur `LoadLast` échoué ; confirm/`--force` sur `--kill-all` (`main.go:110-114,136-145`) | robustesse | S | Élevé — évite de tuer tous les projets sur un config manquant |
| **P0** | Surfacer les échecs de `launch()` : itérer `results`, afficher `r.Error` sur stderr, exit non-zero sur échec partiel (`main.go:449-467`) | errors | S | Élevé — fin du « tout va bien » mensonger |
| **P1** | Ajouter champ `Error` + check `StatusCode`/`isError` à `mpcResponse`/`call()` (`client.go:40-100`) | bugs | S | Élevé — dispatch/Health/doctor cessent de mentir |
| **P1** | `ConfigureAgentsAsync` retourne `error` ; main.go affiche l'échec + le `logPath` (`runner.go:81,182,191`) | errors | S | Élevé — visibilité config |
| **P1** | tmux : distinguer `LookPath` failure (absent) de `ExitError` (0 sessions) ; propager + preflight dans `launch()` (`tmux.go:85-88,106-109`, `main.go:418`) | robustesse | M | Élevé |
| **P1** | Câbler `fleet stop` au relay : `Client.DeactivateAgent` + poll borné sur `HasSession` (`main.go:344-358`) | features | M | Élevé — fin des agents fantômes |
| **P1** | `fleet add` : `Validate()` + réutiliser le bloc relay (register/profile_slug/color) (`main.go:285-329`) | features | M | Élevé |
| **P1** | Extraire `buildConfigureScript(cfg) (string, error)` pur, `json.Marshal` au lieu du JSON hand-built, tests + `bash -n` (`runner.go:79-192`) | tests/sécu | M | Élevé |
| **P1** | Ligne de statut/erreur dans le wizard (`wizard_model.go:76,119,149`) | ux | M | Moyen-élevé |
| **P2** | Status relay-backed : `ListAgents` + `list_tasks`/`get_inbox` au lieu du glyphe `❯` (`main.go:147-181`) | features | L | Moyen |
| **P2** | `fleet usage` + compteur idle-vs-polling + vault bytes injectés | features | M | Moyen |
| **P2** | Flags `--relay-url` / `--force` + champ RelayURL wizard | features | M | Moyen |
| **P2** | Seam `var execCommand = exec.Command` dans runner/doctor + tests argv | tests | M | Moyen |
| **P2** | Remplacer curl inline par `relay.Client` typé ; supprimer le code mort (`runner.go:108-167`, `client.go:144-196`) | arch | L | Moyen |
| **P2** | `runtime.GOOS` dans doctor (brew→apt) + skip iTerm2 off-darwin | portabilité | S | Faible-moyen |
| **P2** | `logs -f` : header + hint Ctrl-C, `\033[H` au lieu de `\033[2J`, erreur non-nil sur session morte | ux | S | Faible-moyen |
| **P2** | Supprimer `extractProject`, grouper via config (`main.go:187-205`) ; palette canonique unique | arch | M | Faible |

## 4. Quick wins (meilleur ratio impact/effort, à faire en premier)

1. **Tuer le `/relay talk` auto + gater sur `AutoTalk`** — `internal/runner/runner.go:169-172` (et `:208,:223`). Effort S, impact critique.
2. **Valider `Project.Name` et `Role`** — `internal/config/config.go:77` (regex `validName` déjà à `config.go:69`) + normaliser le basename à `internal/wizard/project_panel.go:261`. Effort S, ferme RCE + path traversal.
3. **Bloquer l'escalade `--kill` → `--kill-all`** — `cmd/fleet/main.go:110-114`. Effort S, supprime le pire blast radius.
4. **Surfacer les échecs de launch** — `cmd/fleet/main.go:449-467`. Effort S.
5. **Modéliser les erreurs relay** — `internal/relay/client.go:40-100` (champ `Error` + check `StatusCode`/`isError`). Effort S, impacte 4 findings high d'un patch.
