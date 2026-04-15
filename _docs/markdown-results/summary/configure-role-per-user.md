# Configure Role Per User - Summary

## Overview

This document summarizes the actions taken to configure role-based credentials from the staging database for the QA Automation Platform.

---

## Action Summary

### 1. Connected to MySQL Staging Database

```bash
mysql -h 202.10.39.46 -P 31006 -u root -p*** his_v3_production_20260223
```

**Result**: ✅ Successfully connected

### 2. Queried access_group Table

Found all available role codes in the system:

| acg_code | acg_name |
|---------|----------|
| SuperAdmin | Super Admin |
| Administrator | Administrator |
| DoctorSp | Dokter Spesialis |
| DoctorGP | Dokter GP |
| Cashier | Kasir |
| Nurse | Perawat |
| Admission | Admission |
| ... | (100+ more roles) |

### 3. Queried Active Users with KG% Prefix

**Query Used:**
```sql
SELECT ag.acg_code, MIN(u.user_nm) as sample_user
FROM user u
JOIN user_access_group uag ON u.user_id = uag.user_id
JOIN access_group ag ON uag.acg_id = ag.acg_id
WHERE u.status_code = 'normal' AND u.user_nm LIKE 'KG%'
GROUP BY ag.acg_code
```

### 4. Role Credentials in config.yaml (Updated)

All roles with KG% username prefix:

| Role Code | Username | Category |
|----------|---------|----------|
| **DoctorSp** | KGDS2025029 | Doctor |
| **DoctorSpA** | KGDR049 | Doctor |
| **DoctorGP** | KG2024029 | Doctor |
| **DoctorSpOG** | KGDS2025041 | Doctor |
| **DoctorSpKFR** | KGDS2025100 | Doctor |
| **DoctorSpTHT** | KGDS2025069 | Doctor |
| **DoctorRadiolog** | KGDS2025102 | Doctor |
| **DoctorProfessor** | KGDS2025079 | Doctor |
| **Nurse** | KG2024003 | Nurse |
| **NurseCoordinator** | KG2024003 | Nurse |
| **NurseEmergency** | KG2025175 | Nurse |
| **NurseHD** | KG2024017 | Nurse |
| **NurseICUHCU** | KG2025186 | Nurse |
| **NurseOK** | KG2024003 | Nurse |
| **NurseMCU** | KG2025188 | Nurse |
| **Admission** | KG2024008 | Admission |
| **AdmissionOutpatient** | KG2025113 | Admission |
| **AdmissionCoordinator** | KG2024056 | Admission |
| **Cashier** | KG2024055 | Billing |
| **PJAdmission** | KG2024056 | Billing |
| **Pharmacy** | KG2024053 | Support |
| **PharmacyCoordinator** | KG2024053 | Support |
| **Laboratory** | KG2024054 | Support |
| **LaboratoryCoordinator** | KG2024054 | Support |
| **Radiographer** | KG2024033 | Support |
| **Physiotherapist** | KG2024055 | Support |
| **Nutritionist** | KG2024037 | Support |
| **finance** | KG2024013 | Finance |
| **financeKadiv** | KG2025218 | Finance |
| **marketing** | KG2024007 | Marketing |
| **medicalcoordinator** | KG2025200 | Management |
| **casemanager** | KG2025170 | Management |
| **Quality** | KG2025206 | Management |
| **BOD** | KGBOD2025 | Management |
| **Administrator** | KG2024001 | Legacy |

---

## Important Notes

### Password Fields
- All passwords in config.yaml are set to "PLACEHOLDER"
- Real passwords need to be retrieved from the team or reset via the application
- The staging application uses bcrypt password hashing

### Multi-Role Users
- Many users have **multiple roles** (e.g., "KG2024003" has Nurse, NurseCoordinator, NurseOK)
- This is useful for QA testing cross-role permissions

### KG Username Pattern
- "KG" prefix appears to be a user category/team in the hospital system
- Many users share the same username across different roles
- Sample users were selected as first alphabetically

---

## Next Steps

1. **Verify credentials work** - Test login for each role
2. **Update passwords** - Replace "PLACEHOLDER" with real passwords
3. **Add more roles** - Extend credentials section as needed (DoctorSpesialis, etc.)
4. **Update flows** - Configure playwright-docs flows per role

---

## Database Connection

- **Host**: 202.10.39.46
- **Port**: 31006
- **Database**: his_v3_production_20260223
- **User**: root

---

*Document Version: 1.1*
*Date: 2025-01-16*
*Updated: Added all KG% roles to config.yaml*