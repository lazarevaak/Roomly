const victoriaLogs = process.env.VICTORIA_LOGS_URL || 'http://127.0.0.1:9428';
const vmalert = process.env.VMALERT_URL || 'http://127.0.0.1:8880';
const alertmanager = process.env.ALERTMANAGER_URL || 'http://127.0.0.1:9093';

async function getJson(url) {
  const response = await fetch(url, { signal: AbortSignal.timeout(5000) });
  if (!response.ok) throw new Error(`${url}: HTTP ${response.status}`);
  return response.json();
}

const entry = {
  timestamp: new Date().toISOString(),
  level: 'ERROR',
  message: 'Synthetic error for the homework vmalert smoke test',
  'service.name': 'meeting-rooms',
  source: 'bonus-smoke-test',
  test: true,
};
const response = await fetch(`${victoriaLogs}/insert/jsonline?_stream_fields=service.name&_msg_field=message&_time_field=timestamp`, {
  method: 'POST',
  headers: { 'Content-Type': 'application/stream+json' },
  body: `${JSON.stringify(entry)}\n`,
  signal: AbortSignal.timeout(5000),
});
if (!response.ok) throw new Error(`Insert: HTTP ${response.status}: ${await response.text()}`);
console.log('Inserted a synthetic test event directly into VictoriaLogs:');
console.log(JSON.stringify(entry, null, 2));

const deadline = Date.now() + 120000;
let previousState;
while (Date.now() < deadline) {
  const rules = await getJson(`${vmalert}/api/v1/rules`);
  const rule = rules.data.groups.flatMap(group => group.rules)
    .find(rule => rule.name === 'MeetingRoomsErrorLogs');
  if (!rule) throw new Error('MeetingRoomsErrorLogs rule is missing');
  if (rule.health !== 'ok' || rule.lastError) throw new Error(JSON.stringify(rule));
  if (rule.state !== previousState) {
    console.log(`vmalert state: ${rule.state}`);
    previousState = rule.state;
  }
  if (rule.state === 'firing') {
    const alerts = await getJson(`${alertmanager}/api/v2/alerts`);
    const delivered = alerts.find(alert => alert.labels.alertname === 'MeetingRoomsErrorLogs'
      && alert.status.state === 'active');
    if (delivered) {
      console.log('PASS: vmalert is firing and Alertmanager received the alert.');
      console.log(JSON.stringify({ checkedAt: new Date().toISOString(), rule, delivered }, null, 2));
      process.exit(0);
    }
  }
  await new Promise(resolve => setTimeout(resolve, 2000));
}
throw new Error('Timed out waiting for firing and delivery to Alertmanager');
