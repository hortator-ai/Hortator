"""Tests for prompt.py — iteration section."""

from prompt import build_system_prompt, _iteration_section


def test_iteration_section_disabled_when_single_shot():
    result = _iteration_section(1, 1)
    assert result == ""


def test_iteration_section_first_iteration():
    result = _iteration_section(1, 5)
    assert "Planning Loop (Iteration 1/5)" in result
    assert "first iteration" in result
    assert "Plan" in result


def test_iteration_section_middle_iteration():
    result = _iteration_section(3, 5)
    assert "Planning Loop (Iteration 3/5)" in result
    assert "continuing from a previous iteration" in result
    assert "Review" in result


def test_iteration_section_final_iteration():
    result = _iteration_section(5, 5)
    assert "Planning Loop (Iteration 5/5)" in result
    assert "FINAL iteration" in result
    assert "no more iterations" in result


def test_build_system_prompt_includes_iteration():
    prompt = build_system_prompt(
        role="planner",
        tier="tribune",
        capabilities=["spawn"],
        tool_names=["spawn_task"],
        iteration=2,
        max_iterations=5,
    )
    assert "Planning Loop (Iteration 2/5)" in prompt


def test_build_system_prompt_no_iteration_by_default():
    prompt = build_system_prompt(
        role="worker",
        tier="legionary",
        capabilities=[],
        tool_names=[],
    )
    assert "Planning Loop" not in prompt


if __name__ == "__main__":
    import sys
    failures = 0
    for name, fn in list(globals().items()):
        if name.startswith("test_") and callable(fn):
            try:
                fn()
                print(f"  PASS {name}")
            except AssertionError as e:
                print(f"  FAIL {name}: {e}")
                failures += 1
    sys.exit(1 if failures else 0)
