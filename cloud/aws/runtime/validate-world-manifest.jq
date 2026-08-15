def whole_number:
  type == "number" and . == floor;

def safe_id:
  type == "string" and
  length >= 1 and length <= 64 and
  test("^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$");

def safe_peer_id:
  type == "string" and
  length >= 1 and length <= 64 and
  test("^[A-Za-z0-9._-]+$");

def safe_world_name:
  type == "string" and
  length >= 1 and length <= 128 and
  test("^[A-Za-z0-9][A-Za-z0-9 ._()-]*$") and
  (test("[ .]$") | not);

def safe_s3_save_key:
  type == "string" and
  length >= 5 and length <= 1024 and
  endswith(".zip") and
  test("^[A-Za-z0-9][A-Za-z0-9._/-]*$") and
  (split("/") | all(.[]; test("^[A-Za-z0-9][A-Za-z0-9._-]*$")));

def safe_parameter:
  type == "string" and
  length >= 2 and length <= 1011 and
  test("^/[A-Za-z0-9][A-Za-z0-9_.-]*(/[A-Za-z0-9][A-Za-z0-9_.-]*)*$");

def safe_position:
  type == "string" and
  test("^(0|[1-9][0-9]{0,6}),(0|[1-9][0-9]{0,6})$") and
  (split(",") | map(tonumber) | all(.[]; . >= 0 and . <= 1000000));

def allowed_keys($allowed):
  ((keys_unsorted - $allowed) | length) == 0;

def unique_values($values):
  ($values | length) == ($values | unique | length);

type == "object" and
allowed_keys(["schema", "worlds"]) and
.schema == 1 and
(.worlds | type == "array" and length >= 1 and length <= 100) and
all(.worlds[];
  type == "object" and
  allowed_keys([
    "id", "peerId", "worldName", "sidecarPort", "saveKey",
    "credentialParameter", "position", "preferredSlot", "targetTimeScale",
    "saveMinutes", "saveKeep", "enabled"
  ]) and
  has("id") and (.id | safe_id) and
  has("peerId") and (.peerId | safe_peer_id) and
  has("worldName") and (.worldName | safe_world_name) and
  has("sidecarPort") and
    (.sidecarPort | whole_number and . >= 1024 and . <= 65535) and
  has("saveKey") and (.saveKey | safe_s3_save_key) and
  has("credentialParameter") and (.credentialParameter | safe_parameter) and
  ((has("position") | not) or (.position | safe_position)) and
  ((has("preferredSlot") | not) or
    (.preferredSlot | whole_number and . >= 1 and . <= 1000000)) and
  ((has("targetTimeScale") | not) or
    (.targetTimeScale | type == "number" and . >= 0.1 and . <= 1000)) and
  ((has("saveMinutes") | not) or
    (.saveMinutes | type == "number" and . >= 0 and . <= 1440)) and
  ((has("saveKeep") | not) or
    (.saveKeep | whole_number and . >= 0 and . <= 1000)) and
  ((has("enabled") | not) or (.enabled | type == "boolean"))
) and
unique_values([.worlds[].id]) and
unique_values([.worlds[].peerId]) and
unique_values([.worlds[].sidecarPort]) and
unique_values([.worlds[].credentialParameter]) and
unique_values([.worlds[] | select(has("position")) | .position]) and
unique_values([.worlds[] | select(has("preferredSlot")) | .preferredSlot])
