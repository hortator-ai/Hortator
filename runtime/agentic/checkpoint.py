"""
Checkpoint/restore for the reincarnation model.

Saves agent state to /memory/state.json before exiting.
Loads state on startup if a previous checkpoint exists.
"""

import json
import os


def load_checkpoint(state_file: str) -> dict | None:
    """Load a checkpoint from disk, or return None if no checkpoint exists."""
    if not os.path.isfile(state_file):
        return None

    try:
        with open(state_file) as f:
            data = json.load(f)
        if not isinstance(data, dict):
            return None
        if data.get("version") != 1:
            print(f"[hortator-agentic] WARN: unknown checkpoint version: {data.get('version')}")
            return None
        return data
    except (json.JSONDecodeError, OSError) as e:
        print(f"[hortator-agentic] WARN: failed to load checkpoint: {e}")
        return None


def save_checkpoint(state_file: str, state: dict):
    """Save checkpoint state to disk."""
    state.setdefault("version", 1)
    try:
        os.makedirs(os.path.dirname(state_file), exist_ok=True)
        with open(state_file, "w") as f:
            json.dump(state, f, indent=2)
        print(f"[hortator-agentic] Checkpoint saved to {state_file}")
    except OSError as e:
        print(f"[hortator-agentic] ERROR: failed to save checkpoint: {e}")
