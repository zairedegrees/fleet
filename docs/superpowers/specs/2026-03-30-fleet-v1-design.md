# Fleet v0.1 -> v1.0 Design Spec

**Date:** 2026-03-30
**Status:** Approved
**Constraint:** Ship fast + fiable. Pas de cosmétique, mais ce qui ship marche sans surprises.
**Prod safety:** `main` = prod (en cours d'utilisation). Tout le dev sur branche `dev`, merge sprint par sprint.

---

## Structure

4 axes parallèles, 3 sprints séquentiels.

- **Axe 1 — Fleet (CLI)** : 3 sprints, chaque sprint est merge-able indépendamment
- **Axe 2 — wrai.th (spec séparée)** : liste des APIs relay manquantes, à traiter en parallèle
- **Axe 3 — Optimisation** : intégré dans chaque sprint (pas de sprint dédié)
- **Axe 4 — Token Management** : gestion intelligente des tokens pour réduire le coût idle et optimiser le contexte agent

---

## Axe 1 : Fleet CLI

### Sprint 1 — Fiabilité (P0)

Objectif : rendre fleet fiable pour l'usage quotidien. Pas de nouvelles features, que du fix.

#### 1.1 Couleurs agents

**Problème:** `agents_step.go` hardcode `Color: "green"` pour tous les agents récupérés du relay.

**Solution:**
- Lire le champ couleur du profil relay (`Agent.Color` si exposé) ou du TOML sauvé
- Fallback : si pas de couleur, assigner cycliquement depuis la palette existante (green, orange, blue, red, purple, pink, cyan, yellow)
- Le wizard affiche la couleur réelle dans le confirm step

**Fichiers:** `internal/wizard/agents_step.go`, `internal/relay/client.go` (si besoin d'enrichir le parsing)

#### 1.2 `fleet --last` fiable

**Problème:** `runLast()` charge le TOML et lance aveuglément. Pas de vérification d'état.

**Solution:**
- Charger le TOML via `LoadLast()`
- Health check relay (réutilise 1.4)
- Pour chaque agent : `HasSession()` → si existe, proposer skip/kill+restart
- Si tous existent et tournent : message "Fleet already running" + proposer attach
- Si le TOML n'existe pas : message clair + redirect vers wizard

**Fichiers:** `cmd/fleet/main.go` (`runLast`), `internal/runner/runner.go`

#### 1.3 Détection agents existants

**Problème:** `CreateSessions()` skip silencieusement si une session tmux existe. Pas de feedback.

**Solution:**
- Avant `CreateSessions()`, scanner toutes les sessions `pm-*`
- Pour chaque conflit : prompt interactif (attach / skip / kill+restart)
- Option `--force` pour skip les prompts (kill+restart tout)
- Retourner un `LaunchResult` enrichi avec le statut par agent (created/skipped/restarted)

**Fichiers:** `internal/runner/runner.go`, `internal/runner/tmux.go`, `cmd/fleet/main.go`

#### 1.4 Health check pre-launch

**Problème:** Si le relay est down, `launch()` part quand même. Le script async échoue silencieusement.

**Solution:**
- Nouveau check dans `launch()` avant toute action : `relay.Health()` avec timeout 5s
- Si échec : message "Relay unreachable at {url}" + suggestion `fleet --doctor`
- Abort propre (pas de sessions tmux orphelines)
- Vérifier aussi que `tmux` est dispo (réutilise doctor logic)

**Fichiers:** `cmd/fleet/main.go`, `internal/relay/client.go` (ajouter timeout au HTTP client)

#### 1.5 Relay URL configurable

**Problème:** `main.go` hardcode `http://localhost:8090/mcp`.

**Solution:**
- Lire `Project.RelayURL` du TOML chargé
- Default : `http://localhost:8090/mcp` si absent (backward compat)
- Le wizard expose un champ optionnel dans le project step (pré-rempli avec le default)
- `--relay-url` flag CLI en override

**Fichiers:** `cmd/fleet/main.go`, `internal/config/config.go` (champ déjà présent dans `ProjectConfig`), `internal/wizard/project_step.go`

#### 1.6 Fleet-controlled wake (Token Management)

**Problème:** Aujourd'hui `ConfigureAgentsAsync()` envoie `/relay talk` automatiquement aux agents non-exec. Les agents poll `get_session_context()` toutes les 30s même quand idle. Coût : ~2.88M tokens/agent/jour idle, ~17M tokens/jour pour 6 agents.

**Solution:**
- **Supprimer le `/relay talk` automatique** du script de configuration async
- Les agents boot dans un état "registered but idle" : session tmux active, Claude prêt, relay registered, mais pas de talk loop
- Fleet wake un agent quand il a du travail : `SendKeys(agent, "/relay talk")` au moment du dispatch
- Le talk loop s'arrête naturellement après 3 checks vides consécutifs (~1.5 min sans travail)
- Nouveau flag TOML optionnel `AutoTalk = true` pour les agents qui doivent poll en continu (cas rare, backward compat)

**Mécanisme wake :**
```
fleet dispatch <task> --to <agent>
  → relay.DispatchTask(agent, task)
  → tmux.SendKeys(agent, "/relay talk")
  → agent poll, voit la tâche, exécute, finit
  → talk loop meurt seul (3 empty checks)
```

**Impact :** Agents idle = 0 tokens. Agents actifs = tokens uniquement pendant le travail. Économie estimée : ~95% sur les tokens idle.

**Fichiers:** `internal/runner/runner.go` (supprimer `/relay talk` du script async), `cmd/fleet/main.go` (nouveau flow dispatch+wake)

#### Sprint 1 — Optimisation intégrée

- **Config validation** : valider noms d'agents (alphanum + tirets), chemins (existe), couleurs (dans la palette)
- **HTTP timeout** : `relay.Client` avec `http.Client{Timeout: 10 * time.Second}`
- **Tests** : tests unitaires pour health check, config validation, session detection

---

### Sprint 2 — Commandes (P1)

Objectif : enrichir le CLI avec les commandes de gestion runtime.

#### 2.1 `fleet logs <agent>`

**Problème:** Pas de moyen de voir ce qu'un agent fait sans `tmux attach`.

**Solution:**
- Nouvelle commande cobra `logs`
- Arg requis : nom de l'agent (sans préfixe `pm-`)
- Comportement : `CapturePane()` en boucle (poll 1s), affiche les nouvelles lignes
- `--follow` (default true) : mode tail continu
- `--lines N` (default 50) : nombre de lignes initiales
- Ctrl+C pour quitter

**Fichiers:** `cmd/fleet/main.go` (nouvelle commande), `internal/runner/tmux.go` (réutilise `CapturePane`)

#### 2.2 `fleet add <agent>`

**Problème:** Pour ajouter un agent, il faut relancer toute la fleet.

**Solution:**
- Nouvelle commande cobra `add`
- Mini-wizard inline : nom, rôle, couleur, reports-to (réutilise le form de `agents_step.go`)
- Crée la session tmux + lance claude
- Configure via relay (register, color, talk si non-exec)
- Append au TOML courant (recharge `last.toml`)
- Note : pas de re-layout iTerm2 (trop complexe, l'utilisateur fait `Cmd+D` manuellement)

**Fichiers:** `cmd/fleet/main.go`, `internal/runner/runner.go` (extraire `createAndConfigureSingle()`), `internal/config/config.go`

#### 2.3 `fleet stop <agent>`

**Problème:** Pas de moyen propre d'arrêter un seul agent.

**Solution:**
- Nouvelle commande cobra `stop`
- Arg requis : nom de l'agent
- Séquence : envoyer `/exit` via `SendKeys()` → attendre 5s → `KillSession()` si toujours vivant
- Retirer du TOML runtime
- Si c'est le dernier agent : proposer `fleet --kill` (cleanup complet)

**Fichiers:** `cmd/fleet/main.go`, `internal/runner/tmux.go`, `internal/config/config.go`

#### 2.4 Fleet-driven vault injection (Token Management)

**Problème:** Les vault docs et memories doivent être poussés manuellement. Pas de filtre par rôle — chaque agent pourrait charger tout le vault projet, gaspillant du contexte.

**Solution — Fleet décide ce que chaque agent reçoit :**

Structure convention dans le projet :
```
{cwd}/.fleet/
  vault/
    shared/           ← tous les agents reçoivent ça
      architecture.md
      conventions.md
    dev/              ← seulement l'agent dev
      api-guide.md
    auditor/          ← seulement l'agent auditor
      test-strategy.md
    ops/              ← seulement l'agent ops
      deploy-runbook.md
```

**Mécanisme d'injection :**
1. Après relay register, fleet lit `{cwd}/.fleet/vault/`
2. Pour chaque agent : push `shared/*` + `{agent-name}/*` + `{agent-role}/*`
3. Les dossiers qui ne matchent pas le nom/rôle de l'agent sont ignorés
4. Appel relay : `register_vault` ou `set_memory` par doc
5. Si le dossier `.fleet/vault/` n'existe pas : skip silencieusement

**Budget contexte :**
- Chaque doc injecté consomme du contexte initial de l'agent
- Fleet log le nombre de docs et taille totale injectée par agent
- Warning si total > 50KB par agent (risque de saturation contexte)

**Fichiers:** `internal/runner/runner.go`, `internal/relay/client.go` (nouveau call `PushVaultDoc`), nouveau `internal/config/vault.go` (logique de matching rôle/nom)

#### 2.5 Error reporting async

**Problème:** `configure-agents.sh` échoue silencieusement. Aucun feedback.

**Solution:**
- Le script redirige stderr vers `~/.fleet/logs/configure-{timestamp}.log`
- Après launch, fleet attend 5s puis vérifie le log
- Si erreurs détectées : affiche un warning avec `fleet logs --configure` pour voir le détail
- Rotation : garder les 5 derniers logs, supprimer les anciens

**Fichiers:** `internal/runner/runner.go` (`ConfigureAgentsAsync`), `cmd/fleet/main.go`

#### Sprint 2 — Optimisation intégrée

- **Script async → Go natif** : remplacer la génération bash par des goroutines Go avec error handling propre
- **Input validation wizard** : noms d'agents uniques, chemins valides, couleurs dans la palette
- **Tests** : tests pour chaque nouvelle commande (logs, add, stop)

---

### Sprint 3 — Onboarding (P1)

Objectif : fleet sur un projet vierge génère automatiquement une fleet adaptée.

#### 3.1 Scanner de repo

**Nouveau package:** `internal/scanner`

**Détection:**
- Langages : présence de `go.mod`, `package.json`, `Cargo.toml`, `requirements.txt`, `Gemfile`, `pom.xml`, etc.
- Frameworks : `next.config.*`, `vue.config.*`, `angular.json`, `flask`, `django`, etc.
- Structure : `src/`, `test/`, `docs/`, `deploy/`, `.github/`, `Dockerfile`, `Makefile`, `terraform/`
- Taille : nombre de fichiers, LOC approximatif (pour calibrer le nombre d'agents)

**Output:** `ScanResult` struct avec langages, frameworks, structure, taille estimée

**Fichiers:** `internal/scanner/scanner.go`

#### 3.2 Mapping stack -> agents

**Nouveau fichier:** `internal/scanner/profiles.go`

**Règles de mapping:**
| Stack détecté | Agents proposés |
|---------------|----------------|
| Tout projet | `dev` (toujours) |
| Tests présents | `+ auditor` |
| `docs/` ou README conséquent | `+ docs` |
| Frontend (React, Vue, Svelte) | `+ frontend`, `+ ux-designer` |
| Infra (Docker, Terraform, CI) | `+ ops` |
| Data/ML (notebooks, models) | `+ researcher` |
| Monorepo (>3 packages) | `+ architect` |
| Finance/trading | `+ quant` |

Chaque profil proposé inclut : nom, rôle, couleur (pré-assignée), reports_to (dev par défaut).

L'utilisateur peut override tout dans le wizard.

#### 3.3 Wizard intégration

**Nouveau step:** `internal/wizard/scan_step.go`

**Flow modifié:**
1. `runProjectStep()` — si "Create new..." sélectionné
2. `runCwdStep()` — pick le répertoire
3. **`runScanStep()`** — NEW : scanne le cwd, affiche résultat, propose agents
4. `runAgentsStep()` — pré-rempli avec les agents proposés, l'utilisateur édite
5. `runConfirmStep()` — summary + launch

**UX du scan step:**
```
Scanning /path/to/project...

Detected:
  Languages: Go, TypeScript
  Frameworks: Cobra CLI, React
  Structure: src/, test/, docs/, .github/

Suggested agents:
  dev (green)      — Lead developer
  auditor (orange) — Code review & testing
  frontend (blue)  — React UI development
  ops (purple)     — CI/CD & deployment

Press Enter to continue with these agents, or edit in next step.
```

#### 3.4 Create profiles relay

**Après validation wizard, pour chaque agent proposé :**
1. Vérifier si le profil existe déjà sur le relay (`ListProfiles`)
2. Si non : `relay.RegisterProfile(agent)` avec slug, name, role
3. Si oui : skip (pas d'overwrite)

**Dépendance relay :** nécessite `register_profile` API. Si indisponible, skip avec warning.

**Fichiers:** `internal/relay/client.go` (nouveau call), `internal/runner/runner.go`

#### Sprint 3 — Optimisation intégrée

- **Cache scan** : sauver le `ScanResult` dans le TOML pour ne pas re-scanner à chaque launch
- **Tests** : tests pour le scanner (fixtures de repos types), tests pour le mapping

---

## Axe 2 : Besoins wrai.th (spec séparée)

APIs relay nécessaires pour fleet v1, classées par sprint de dépendance.

### Haute priorité (Sprint 1-2)

| API | Méthode relay | Usage fleet | Sprint |
|-----|---------------|-------------|--------|
| Health check fiable | `health` ou `list_orgs` avec timeout | Pre-launch validation | 1 |
| Agent color | Enrichir `list_agents` response avec `color` | Couleurs wizard | 1 |
| Vault push | `register_vault` + `set_memory` | Vault auto-inject | 2 |
| Agent deregister | `deactivate_agent` ou `delete_agent` | `fleet stop` | 2 |

### Moyenne priorité (Sprint 3)

| API | Méthode relay | Usage fleet | Sprint |
|-----|---------------|-------------|--------|
| Profile create | `register_profile` | Onboarding auto-create | 3 |
| Bulk vault push | Batch `register_vault` | Onboarding efficient | 3 |
| Agent status (idle/busy) | `list_agents` enrichi ou nouveau endpoint | Dashboard futur (P2) | post-v1 |

### Note

Fleet doit fonctionner même si ces APIs relay ne sont pas encore disponibles. Chaque feature qui dépend d'une API relay doit avoir un **fallback gracieux** : skip avec warning, pas crash.

---

## Axe 3 : Optimisation (intégrée)

Pas de sprint dédié. Chaque sprint inclut ses optimisations.

| Item | Sprint | Détail |
|------|--------|--------|
| HTTP timeout relay | 1 | `http.Client{Timeout: 10s}` |
| Config validation | 1 | Noms alphanum+tirets, chemins existants, couleurs valides |
| Test coverage | 1-3 | Tests unitaires par feature |
| Script bash → Go natif | 2 | Remplacer `configure-agents.sh` par goroutines |
| Input validation wizard | 2 | Noms uniques, longueur max, caractères interdits |
| Scan result cache | 3 | Éviter re-scan inutile |

---

## Axe 4 : Token Management

### Contexte

Audit du 2026-03-30 : une fleet de 6 agents idle consomme ~17M tokens/jour via le talk loop polling (`get_session_context()` toutes les 30s × 6 agents). Les vault docs et memories sont accessibles à tous sans filtre par rôle, ce qui pousse les agents à charger du contexte inutile.

### Principes

1. **Agents idle = 0 tokens.** Pas de polling sans travail. Fleet wake les agents on-demand.
2. **Contexte minimal au boot.** Chaque agent reçoit uniquement les vault docs/memories pertinents à son rôle.
3. **Fleet est le chef d'orchestre.** C'est fleet qui décide quand un agent travaille et ce qu'il charge, pas l'agent lui-même.

### Items

| Item | Sprint | Impact estimé |
|------|--------|---------------|
| Fleet-controlled wake (1.6) | 1 | -95% tokens idle (~17M → ~0 en idle) |
| Fleet-driven vault injection (2.4) | 2 | -60% contexte initial gaspillé |

### Architecture token-aware

```
AVANT (v0.1):
  boot → register → /relay talk → poll 30s forever
  vault: tout accessible, rien filtré
  coût idle: ~2.88M tokens/agent/jour

APRÈS (v1.0):
  boot → register → idle (0 tokens)
  fleet dispatch → wake → talk → travaille → talk loop meurt
  vault: shared/ + {role}/ seulement
  coût idle: 0 tokens/agent/jour
  coût actif: tokens uniquement pendant le travail
```

### Métriques à suivre (post-v1)

- Tokens consommés par agent par jour (via relay billing si disponible)
- Nombre de wake/sleep cycles par agent
- Taille moyenne du vault injecté par rôle
- Ratio tokens utiles (travail) vs overhead (polling, boot, injection)

Ces métriques sont hors scope v1 mais guideront les optimisations futures.

---

## Contraintes techniques

- **Backward compat TOML** : les configs v0.1 (`FleetConfig` actuel) doivent continuer à fonctionner. Nouveaux champs optionnels uniquement.
- **Branche `dev`** : tout le travail. Merge dans `main` sprint par sprint après tests.
- **Prod intouchée** : `main` = ce qui tourne. Aucun commit direct.
- **Pas de breaking changes** sur les commandes existantes (`fleet`, `--last`, `--kill`, `--status`, `--doctor`).
- **macOS only** pour v1 : pas de portage Linux/Windows. iTerm2 + osascript restent.

---

## Hors scope v1

- Dashboard live persistant (P2)
- Multi-machine SSH (P2)
- `brew install` (P2)
- Portage cross-platform
- Plugin system
- Auto-healing (restart agents morts)
