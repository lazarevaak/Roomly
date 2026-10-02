INSERT INTO users (username, password_hash, role)
VALUES (
    'admin',
    'sha256$roomly-local-salt$d1d040912915b988f9cb3623eafc40d7428cf2e1067c67d67d8a1adee0d346fb',
    'admin'
)
ON CONFLICT (username) DO NOTHING;

UPDATE users
SET role = 'admin'
WHERE username = 'admin';
