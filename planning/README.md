# Planning

Use this directory for plan documents and analyst writeups.

Rules:

- Keep raw captures and bulky evidence out of `planning/`.
- Put pcaps, PMS logs, Tunerr logs, and debug-bundle output under `.diag/evidence/<case-id>/`.
- In plan files here, link to the matching `.diag/evidence/<case-id>/notes.md` and `report.txt`.

Recommended flow:

1. Create the evidence bundle:
   - `scripts/evidence-intake.sh -id <case-id> -print`
2. Fill `.diag/evidence/<case-id>/` with logs, pcap, and debug-bundle output.
3. Run:
   - `python3 scripts/analyze-bundle.py .diag/evidence/<case-id> --output .diag/evidence/<case-id>/report.txt`
4. Write or update the plan in `planning/` with findings and the implementation steps.
