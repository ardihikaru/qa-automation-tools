#!/usr/bin/env python3
import bcrypt

# The common hash found in database
stored_hash = b'$2a$09$WQKyrsr2R/IEpxQrqgt4w.MQ4WkoEuvftJEma22JcKTn4kraTiycO'

# Try common test passwords
test_passwords = [
    'Bismillah1!',
    'password',
    '123456',
    'Password123',
    'P4ssw0rd123',
    'kg2024001',
    'kg2024',
    'password123',
    'admin123',
    'test1234',
    '12345678',
]

for pwd in test_passwords:
    try:
        if bcrypt.checkpw(pwd.encode(), stored_hash):
            print(f'✓ Found match: {pwd}')
            break
    except Exception as e:
        pass
else:
    print('No match found - the password might be unique per user')