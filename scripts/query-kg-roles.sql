SELECT ag.acg_code, MIN(u.user_nm) as sample_user
FROM user u
JOIN user_access_group uag ON u.user_id = uag.user_id
JOIN access_group ag ON uag.acg_id = ag.acg_id
WHERE u.status_code = 'normal' AND u.user_nm LIKE 'KG%'
GROUP BY ag.acg_code
ORDER BY ag.acg_code