SELECT u.user_id, u.username, u.status_code, ag.acg_code, ag.acg_name
FROM user u
JOIN user_access_group uag ON u.user_id = uag.user_id
JOIN access_group ag ON uag.acg_id = ag.acg_id
WHERE u.status_code = 'normal'
AND ag.acg_code IN ('SuperAdmin', 'Administrator', 'DoctorSp', 'DoctorGP', 'Cashier', 'Kasir', 'Nurse', 'Adm RI')
ORDER BY ag.acg_code, u.user_id
LIMIT 20