# Known folders initialization

## Goal

Avoid repeated model discovery for common user locations while keeping the
filesystem—not a user-provided name—the source of truth.

## Design

Add an `/init`-style first-run command that resolves and verifies common
locations for the active Windows user, then stores the results in a local
Markdown manifest. Subsequent requests load those verified paths into the
agent's environment context.

Suggested manifest location: `.underroot/known-folders.md` in the user's
Underroot configuration directory, not the project repository.

## Initial folders

- Desktop
- Downloads
- Documents
- Pictures
- Videos
- Music

Each entry records the display name, absolute path, verification timestamp,
and whether the directory existed when initialized.

## Workflow

1. Add an `init` command to the interactive shell.
2. Resolve known folders through Windows APIs or environment-backed PowerShell,
   never by constructing paths from model output.
3. Verify each resolved path against the filesystem.
4. Write the verified results to the Markdown manifest.
5. Load the manifest at startup and add existing entries to `ENVIRONMENT` as
   trusted facts.
6. If a stored folder is missing, mark it stale and require a fresh resolution
   before it can be used.
7. Keep discovery tools for arbitrary, partial, or misspelled file and folder
   names; the manifest only avoids discovery for known locations.

## Safety and privacy

- Store only local paths and verification metadata—never file listings or
  contents.
- Keep the manifest user-local and exclude it from version control.
- Require the generated script to use only a manifest path that is still
  verified as present.

## Acceptance criteria

- First-run initialization creates a readable known-folders manifest.
- Later requests such as "open Downloads" use the saved, verified path without
  a model tool call.
- A moved or deleted directory is detected and never treated as a valid path.
- Arbitrary path requests still use filesystem discovery as the source of
  truth.
