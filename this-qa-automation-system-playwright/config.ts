// Test configuration and constants
export const config = {
  // Backend API
  // Override with QA_BACKEND_URL (e.g. https://his-v3.qa.sismedika.com)
  backendUrl: process.env.QA_BACKEND_URL || 'http://localhost:8080',
  
  // Dashboard
  // Override with QA_DASHBOARD_URL (e.g. https://his-v3.qa.sismedika.com)
  dashboardUrl: process.env.QA_DASHBOARD_URL || 'http://localhost:3000',
  
  // Timeouts
  apiTimeout: 10000,
  pageTimeout: 30000,
  
  // Expected data
  expectedSkills: ['max_length', 'required_field', 'data_persistence', 'multi_tab_save'],
};