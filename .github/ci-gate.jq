# Normalizza workflow run e check run di un commit per il job "CI gate".
# Input (jq -n): $runs, $checks (da --slurpfile), $self_wf (workflow_id di ci-gate).
# Output: [{kind, name, status, conclusion}], una voce per workflow/check,
# tenendo solo l'esecuzione più recente (re-run, reopen, synchronize).

def latest_by(k): group_by(k) | map(max_by(.id));

(
  $runs
  | map(select(.workflow_id != $self_wf and .event != "dynamic"))
  | latest_by([.workflow_id, .event])
  | map({kind: "workflow", name, status, conclusion})
)
+
(
  $checks
  # I check di GitHub Actions sono già coperti dai workflow run: qui solo app esterne
  # (es. la validazione di .github/dependabot.yml).
  | map(select(.app.slug != "github-actions"))
  | latest_by(.name)
  | map({kind: "check", name, status, conclusion})
)
