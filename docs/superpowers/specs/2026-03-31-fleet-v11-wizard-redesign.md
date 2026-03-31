# Fleet v1.1 — Wizard UX Redesign Spec

**Date:** 2026-03-31
**Status:** Approved
**Goal:** Replace the multi-step wizard with a one-screen split-panel interface featuring preset teams and inline agent editing.
**Constraint:** Ship fast + fiable. Utiliser les composants Bubbles (list, textinput) au lieu du bubbletea artisanal.

---

## Overview

Le wizard v1.0 est un flow multi-step sequentiel (5 etapes) qui prend trop de temps. La v1.1 le remplace par un **one-screen split-panel** :

- **Panel gauche** : project config (name, path) + preset selector
- **Panel droit** : agent list avec toggle/edit/create/delete
- **Bottom drawer** : formulaire d'edition agent (s'ouvre sur `e` ou `n`)

Un preset selectionne pre-remplit les agents d'un click. L'utilisateur peut editer avant de lancer.

---

## Presets

7 presets d'equipe integres :

| Preset | Agents | Description |
|--------|--------|-------------|
| Web App | dev, frontend, ux-designer, auditor, ops | React/Vue/Next.js + backend |
| API / Backend | dev, auditor, ops | API pure, pas de frontend |
| Data / ML | dev, researcher, quant, auditor | Notebooks, pipelines, models |
| Trading Bot | dev, quant, auditor, ops, researcher, ux-designer | Finance + monitoring |
| Full Stack | dev, frontend, ux-designer, auditor, ops, researcher, docs | Equipe complete |
| Minimal | dev, auditor | Pair programming |
| Custom | (vide) | From scratch, scan repo optionnel |

Chaque preset definit une liste d'`AgentConfig` avec name, color (cyclique depuis palette), role, et reports_to (tous report au dev par defaut).

Les presets sont stockes dans un nouveau fichier `internal/wizard/presets.go` — pas de config externe, c'est du code Go.

---

## Layout

### Structure ecran

```
+------------------------------------------+
| ⚡ Fleet Wizard                          |
+------------------+-----------------------+
| PROJECT          | AGENTS (N) — RxC grid |
| Name: my-webapp  | ▸ [x] dev       ● Lead|
| Path: ~/code/... |   [x] frontend  ● FE  |
|                  |   [x] auditor   ● QA  |
| PRESET           |   [x] ops       ● Ops |
| ▸ 🌐 Web App (5)|   + New agent (n)     |
|   ⚙ API (3)     |                       |
|   📊 Data (4)    |                       |
|   💰 Trading (6) |                       |
|   🚀 Full (7)    |                       |
|   ⚡ Minimal (2) |                       |
|   🔧 Custom (0)  |                       |
+------------------+-----------------------+
| tab=panel j/k=move space=toggle e=edit   |
| n=new d=del enter=launch s=save+launch   |
+------------------------------------------+
```

### Bottom Drawer (mode edit)

Quand l'utilisateur appuie `e` sur un agent ou `n` pour nouveau, le panel droit se split :

```
+------------------+-----------------------+
| PROJECT          | AGENTS (compresse)    |
| ...              |  [x] dev [x] fe [!ux] |
| PRESET           +-----------------------+
| ...              | Edit: ux-designer     |
|                  | ▸ Name: ux-designer_  |
|                  |   Role: UX design     |
|                  |   Color: [blue] ...   |
|                  |   Boss:  [dev] ...    |
+------------------+-----------------------+
| tab=field j/k=sel enter=save esc=cancel  |
+------------------------------------------+
```

### Navigation

| Touche | Action | Contexte |
|--------|--------|----------|
| `tab` | Switch entre panel gauche et panel droit | Normal |
| `j`/`k` ou fleches | Monter/descendre dans le panel actif | Normal |
| `space` | Toggle agent on/off | Panel droit, agent selectionne |
| `enter` | Panel gauche: confirmer champ / selectionner preset. Panel droit: **launch fleet** | Normal |
| `s` | Save config + launch | Normal |
| `e` | Ouvrir bottom drawer pour editer l'agent sous le curseur | Panel droit |
| `n` | Ouvrir bottom drawer pour creer un nouvel agent | Panel droit |
| `d` / `delete` | Supprimer l'agent sous le curseur | Panel droit |
| `a` | Toggle tous les agents on/off | Panel droit |
| `tab` (dans drawer) | Champ suivant dans le formulaire | Bottom drawer |
| `j`/`k` (dans drawer) | Naviguer options (color, reports-to) | Bottom drawer, champ select |
| `enter` (dans drawer) | Sauver et fermer le drawer | Bottom drawer |
| `esc` | Fermer le drawer sans sauver / quitter | Bottom drawer / Normal |
| `q` | Quitter fleet | Normal |

### Panneau gauche — Etats

1. **Input mode** (premier lancement) : curseur sur Name, l'utilisateur tape. Tab passe a Path. Enter confirme.
2. **Preset mode** : apres project confirme, curseur navigue les presets. Enter selectionne un preset → pre-remplit les agents dans le panel droit.
3. **Locked mode** : quand le focus est sur le panel droit, le panel gauche affiche les valeurs mais n'est pas interactif.

### Panneau droit — Etats

1. **List mode** : liste des agents avec checkbox toggle. Curseur j/k.
2. **Empty state** : si preset "Custom" ou aucun preset, affiche un message centré "No agents — press `n` to create one or select a preset."
3. **Drawer mode** : bottom drawer ouvert, liste compressée en haut (une ligne, noms seulement).

---

## Architecture

### Fichiers

| Fichier | Action | Responsabilite |
|---------|--------|----------------|
| `internal/wizard/presets.go` | **Nouveau** | Definitions des 7 presets + `GetPreset(name)` |
| `internal/wizard/wizard_model.go` | **Nouveau** | Model bubbletea principal (one-screen), state machine, routing messages |
| `internal/wizard/project_panel.go` | **Nouveau** | Panel gauche : project input + preset list |
| `internal/wizard/agents_panel.go` | **Nouveau** | Panel droit : agent list + toggle |
| `internal/wizard/agent_drawer.go` | **Nouveau** | Bottom drawer : formulaire edit/create agent |
| `internal/wizard/wizard.go` | **Modifier** | `Run()` remplace par le nouveau model one-screen |
| `internal/wizard/project_step.go` | **Supprimer** | Remplace par project_panel.go |
| `internal/wizard/agents_step.go` | **Supprimer** | Remplace par agents_panel.go |
| `internal/wizard/confirm_step.go` | **Supprimer** | Plus de step confirm, enter = launch |
| `internal/wizard/cwd_step.go` | **Supprimer** | Integre dans project_panel.go |
| `internal/wizard/scan_step.go` | **Supprimer** | Scan integre dans le preset "Custom" |
| `internal/wizard/layout_step.go` | **Conserver** | `autoLayout()` reutilise pour le grid display |

### Model Bubbletea

```go
type wizardModel struct {
    // Layout
    width, height int
    activePanel   panel // panelLeft, panelRight

    // Left panel
    projectName   textinput.Model
    projectPath   textinput.Model
    projectReady  bool        // true apres Enter sur les champs
    presets       []preset
    presetCursor  int
    leftFocus     leftFocus   // focusName, focusPath, focusPresets

    // Right panel
    agents       []agentItem
    agentCursor  int

    // Bottom drawer
    drawerOpen   bool
    drawerMode   drawerMode  // drawerEdit, drawerCreate
    drawerFields [4]drawerField // name, role, color, reportsTo
    drawerCursor int         // field index 0-3
    editingIdx   int         // index of agent being edited (-1 for new)

    // State
    relayClient  *relay.Client
    quitting     bool
    launching    bool
    saving       bool
}
```

### State Machine

```
INIT → focusName (typing project name)
  Enter → focusPath (typing project path)
    Enter → focusPresets (navigating presets, panel droit active)
      Enter on preset → agents pre-remplis, focus panel droit
        tab → switch panel
        e → drawerOpen (edit mode)
        n → drawerOpen (create mode)
          Enter in drawer → save, close drawer
          Esc in drawer → cancel, close drawer
        Enter in panel droit → LAUNCH
        s → SAVE + LAUNCH
        q → QUIT
```

### Composants Bubbles utilises

- `textinput.Model` pour project name et path (avec placeholder, cursor blink)
- Pas de `list.Model` pour les presets et agents — trop de customisation necessaire, on garde le rendu custom avec lipgloss. Mais on utilise les **patterns** Bubbles (Update retourne model + cmd).

### Normalisation des noms

Tout nom d'agent tape par l'utilisateur passe par `normalizeName()` (deja implemente) : "UX Designer" → "ux-designer".

### Scan repo

Le scan n'est plus un step separe. Quand l'utilisateur selectionne le preset "Custom", fleet scan le `projectPath` et pre-remplit avec les suggestions du scanner. Si le scan ne trouve rien, la liste reste vide avec le message "press `n`".

### Backward Compat

- `--last` fonctionne toujours (bypass wizard, charge TOML)
- Les TOML v1.0 sont toujours lisibles
- `wizard.Run()` retourne toujours `*WizardResult` avec la meme structure

---

## Rendu Lipgloss

### Couleurs

| Element | Couleur |
|---------|---------|
| Titre "Fleet Wizard" | Bold, color 99 (purple) |
| Curseur actif `▸` | Color 86 (cyan) |
| Texte selectionne / valeurs | Color 86 (cyan) |
| Labels / dim text | Color 241 (gray) |
| Bordure panel actif | Color 99 (purple) |
| Bordure panel inactif | Color 238 (dark gray) |
| Agent dot couleur | La couleur de l'agent (ANSI: green=2, orange=208, blue=4, red=1, purple=5, pink=13, cyan=6, yellow=3) |
| Drawer header | Color 99 (purple), bold |
| Help bar | Color 241 (gray) |

### Responsive

- Terminal >= 80 cols : split panel normal
- Terminal < 80 cols : fallback stack vertical (panel gauche au dessus, panel droit en dessous)
- Detection via `tea.WindowSizeMsg`

---

## Hors scope

- Sauvegarde de presets custom (v1.2)
- Import/export de configs (v1.2)
- Preview iTerm2 grid dans le wizard (v1.2)
- Animation de transition entre modes (YAGNI)
