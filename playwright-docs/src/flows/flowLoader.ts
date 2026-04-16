/**
 * Flow definitions for different roles - Updated for HIS Frontend
 * Fixed login path and selectors to match his-frontend
 */

export interface FlowStep {
  action: 'click' | 'type' | 'select' | 'wait' | 'navigate';
  selector?: string;
  value?: string;
  wait?: number;
}

export interface FlowDefinition {
  id: string;
  name: string;
  role: 'admin' | 'doctor' | 'cashier' | 'super_admin' | 'nurse' | 'admission' | 'pharmacy' | 'laboratory';
  description: string;
  steps: FlowStep[];
}

// Helper function to create login steps with correct HIS frontend selectors
function createLoginSteps(username: string, password: string): FlowStep[] {
  return [
    { action: 'navigate', selector: '/auth/login' },
    { action: 'wait', wait: 2000 }, // Wait for page to load
    { action: 'type', selector: 'input[name="username"]', value: username },
    { action: 'wait', wait: 500 },
    { action: 'type', selector: 'input[name="password"]', value: password },
    { action: 'wait', wait: 500 },
    { action: 'click', selector: 'button[type="submit"], button:has-text("Login")' },
    { action: 'wait', wait: 5000 }, // Wait for login to complete
  ];
}

// Super Admin flows (using super_admin credential)
const superAdminFlows: FlowDefinition[] = [
  {
    id: 'super-admin-login',
    name: 'Super Admin Login',
    role: 'super_admin',
    description: 'Login as super administrator',
    steps: createLoginSteps('{{username}}', '{{password}}'),
  },
  {
    id: 'super-admin-dashboard',
    name: 'Super Admin Dashboard',
    role: 'super_admin',
    description: 'View super admin dashboard after login',
    steps: [
      { action: 'navigate', selector: '/administration/encounter/outpatient-dashboard' },
      { action: 'wait', wait: 3000 },
    ],
  },
  {
    id: 'super-admin-user-management',
    name: 'User Management',
    role: 'super_admin',
    description: 'Access user management menu',
    steps: [
      { action: 'navigate', selector: '/administration/user' },
      { action: 'wait', wait: 3000 },
    ],
  },
];

// Admin flows
const adminFlows: FlowDefinition[] = [
  {
    id: 'admin-login',
    name: 'Admin Login',
    role: 'admin',
    description: 'Login as system administrator',
    steps: createLoginSteps('{{username}}', '{{password}}'),
  },
  {
    id: 'admin-dashboard',
    name: 'Admin Dashboard',
    role: 'admin',
    description: 'View admin dashboard',
    steps: [
      { action: 'navigate', selector: '/administration/encounter/outpatient-dashboard' },
      { action: 'wait', wait: 2000 },
    ],
  },
];

// Doctor GP flows
const doctorFlows: FlowDefinition[] = [
  {
    id: 'doctor-gp-login',
    name: 'Doctor GP Login',
    role: 'doctor',
    description: 'Login asGeneral Practitioner doctor',
    steps: createLoginSteps('{{username}}', '{{password}}'),
  },
  {
    id: 'doctor-gp-dashboard',
    name: 'Doctor GP Dashboard',
    role: 'doctor',
    description: 'View doctor dashboard',
    steps: [
      { action: 'navigate', selector: '/doctor/queue' },
      { action: 'wait', wait: 3000 },
    ],
  },
  {
    id: 'doctor-gp-patient-list',
    name: 'View Patient List',
    role: 'doctor',
    description: 'View list of patients',
    steps: [
      { action: 'navigate', selector: '/doctor/patient' },
      { action: 'wait', wait: 3000 },
    ],
  },
  {
    id: 'doctor-gp-medical-record',
    name: 'Medical Record',
    role: 'doctor',
    description: 'Access medical record',
    steps: [
      { action: 'navigate', selector: '/doctor/medical-record' },
      { action: 'wait', wait: 3000 },
    ],
  },
  {
    id: 'doctor-gp-prescription',
    name: 'Prescription',
    role: 'doctor',
    description: 'Create prescription',
    steps: [
      { action: 'navigate', selector: '/doctor/prescription' },
      { action: 'wait', wait: 3000 },
    ],
  },
];

// Nurse flows
const nurseFlows: FlowDefinition[] = [
  {
    id: 'nurse-login',
    name: 'Nurse Login',
    role: 'nurse',
    description: 'Login as nurse',
    steps: createLoginSteps('{{username}}', '{{password}}'),
  },
  {
    id: 'nurse-dashboard',
    name: 'Nurse Dashboard',
    role: 'nurse',
    description: 'View nurse dashboard',
    steps: [
      { action: 'navigate', selector: '/nurse/queue' },
      { action: 'wait', wait: 3000 },
    ],
  },
  {
    id: 'nurse-patient-vitals',
    name: 'Patient Vitals',
    role: 'nurse',
    description: 'Record patient vitals',
    steps: [
      { action: 'navigate', selector: '/nurse/vitals' },
      { action: 'wait', wait: 3000 },
    ],
  },
];

// Admission flows
const admissionFlows: FlowDefinition[] = [
  {
    id: 'admission-login',
    name: 'Admission Login',
    role: 'admission',
    description: 'Login as admission staff',
    steps: createLoginSteps('{{username}}', '{{password}}'),
  },
  {
    id: 'admission-dashboard',
    name: 'Admission Dashboard',
    role: 'admission',
    description: 'View admission dashboard',
    steps: [
      { action: 'navigate', selector: '/administration/encounter/outpatient-dashboard' },
      { action: 'wait', wait: 3000 },
    ],
  },
  {
    id: 'admission-new-patient',
    name: 'New Patient Registration',
    role: 'admission',
    description: 'Register new patient',
    steps: [
      { action: 'navigate', selector: '/administration/patient/new' },
      { action: 'wait', wait: 3000 },
    ],
  },
  {
    id: 'admission-queue',
    name: 'Queue Management',
    role: 'admission',
    description: 'Manage patient queue',
    steps: [
      { action: 'navigate', selector: '/administration/encounter/queue' },
      { action: 'wait', wait: 3000 },
    ],
  },
];

// Cashier flows
const cashierFlows: FlowDefinition[] = [
  {
    id: 'cashier-login',
    name: 'Cashier Login',
    role: 'cashier',
    description: 'Login as cashier',
    steps: createLoginSteps('{{username}}', '{{password}}'),
  },
  {
    id: 'cashier-dashboard',
    name: 'Cashier Dashboard',
    role: 'cashier',
    description: 'View cashier dashboard',
    steps: [
      { action: 'navigate', selector: '/cashier' },
      { action: 'wait', wait: 3000 },
    ],
  },
  {
    id: 'cashier-billing',
    name: 'Billing',
    role: 'cashier',
    description: 'View billing transactions',
    steps: [
      { action: 'navigate', selector: '/cashier/billing' },
      { action: 'wait', wait: 3000 },
    ],
  },
  {
    id: 'cashier-payment',
    name: 'Payment',
    role: 'cashier',
    description: 'Process payment',
    steps: [
      { action: 'navigate', selector: '/cashier/payment' },
      { action: 'wait', wait: 3000 },
    ],
  },
];

// Pharmacy flows
const pharmacyFlows: FlowDefinition[] = [
  {
    id: 'pharmacy-login',
    name: 'Pharmacy Login',
    role: 'pharmacy',
    description: 'Login as pharmacy staff',
    steps: createLoginSteps('{{username}}', '{{password}}'),
  },
  {
    id: 'pharmacy-dashboard',
    name: 'Pharmacy Dashboard',
    role: 'pharmacy',
    description: 'View pharmacy dashboard',
    steps: [
      { action: 'navigate', selector: '/pharmacy' },
      { action: 'wait', wait: 3000 },
    ],
  },
  {
    id: 'pharmacy-dispense',
    name: 'Dispense Medicine',
    role: 'pharmacy',
    description: 'Dispense medicine to patient',
    steps: [
      { action: 'navigate', selector: '/pharmacy/dispense' },
      { action: 'wait', wait: 3000 },
    ],
  },
  {
    id: 'pharmacy-inventory',
    name: 'Inventory',
    role: 'pharmacy',
    description: 'View medicine inventory',
    steps: [
      { action: 'navigate', selector: '/pharmacy/inventory' },
      { action: 'wait', wait: 3000 },
    ],
  },
];

// Laboratory flows
const laboratoryFlows: FlowDefinition[] = [
  {
    id: 'laboratory-login',
    name: 'Laboratory Login',
    role: 'laboratory',
    description: 'Login as laboratory staff',
    steps: createLoginSteps('{{username}}', '{{password}}'),
  },
  {
    id: 'laboratory-dashboard',
    name: 'Laboratory Dashboard',
    role: 'laboratory',
    description: 'View laboratory dashboard',
    steps: [
      { action: 'navigate', selector: '/laboratory' },
      { action: 'wait', wait: 3000 },
    ],
  },
  {
    id: 'laboratory-results',
    name: 'Lab Results',
    role: 'laboratory',
    description: 'View lab results',
    steps: [
      { action: 'navigate', selector: '/laboratory/results' },
      { action: 'wait', wait: 3000 },
    ],
  },
  {
    id: 'laboratory-specimen',
    name: 'Specimen Management',
    role: 'laboratory',
    description: 'Manage specimens',
    steps: [
      { action: 'navigate', selector: '/laboratory/specimen' },
      { action: 'wait', wait: 3000 },
    ],
  },
];

// Radiographer flows (uses laboratory role)
const radiographerFlows: FlowDefinition[] = [
  {
    id: 'radiographer-login',
    name: 'Radiographer Login',
    role: 'laboratory',
    description: 'Login as radiographer',
    steps: createLoginSteps('{{username}}', '{{password}}'),
  },
  {
    id: 'radiographer-dashboard',
    name: 'Radiographer Dashboard',
    role: 'laboratory',
    description: 'View radiographer dashboard',
    steps: [
      { action: 'navigate', selector: '/radiology' },
      { action: 'wait', wait: 3000 },
    ],
  },
];

// Export all flows
export function loadFlows(): FlowDefinition[] {
  return [
    ...superAdminFlows,
    ...adminFlows,
    ...doctorFlows,
    ...nurseFlows,
    ...admissionFlows,
    ...cashierFlows,
    ...pharmacyFlows,
    ...laboratoryFlows,
    ...radiographerFlows,
  ];
}

export function getFlowsByRole(role: string): FlowDefinition[] {
  return loadFlows().filter(f => f.role === role);
}