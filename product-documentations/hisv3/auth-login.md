# HIS V3 Staging Documentation - Login Page

## Page: Authentication
**URL Path**: `/auth/login`

## Description
The login page is the entry point for all users accessing the HIS V3 system. It provides secure authentication using username and password credentials.

## Access
This page is accessible to **all roles** without authentication.

## User Flow

1. Navigate to `/auth/login`
2. Enter username in `input[name="username"]`
3. Enter password in `input[name="password"]`
4. Click submit button (`button[type="submit"]`)
5. Wait for redirect to role-specific dashboard

## Roles That Can Access This Page

| Role | Credential Example | Status |
|------|-------------------|--------|
| Doctor SP | KGDS2025029 | ✅ Working |
| Doctor GP | KG2024029 | ✅ Working |
| Doctor SP OG | KGDS2025041 | ✅ Working |
| Doctor SP KFR | KGDS2025100 | ✅ Working |
| Doctor SP THT | KGDS2025069 | ✅ Working |
| Doctor Professor | KGDS2025079 | ✅ Working |
| Doctor Radiologist | KGDS2025102 | ✅ Working |
| Nurse | KG2024003 | ✅ Working |
| Admission | KG2024008 | ✅ Working |

## Technical Details

- **Authentication Type**: Session-based (cookies)
- **Timeout**: 30 seconds
- **Wait Strategy**: `domcontentloaded`

## Screenshots Location
`playwright-docs/output/screenshots/stag-app-hisv3.sismedika.online/{role_code}/`

## Test Date
2025-04-16

## Notes
- All credentials with "KG" prefix work correctly
- Session persistence is maintained after successful login
- Password default: "Bismillah1!"