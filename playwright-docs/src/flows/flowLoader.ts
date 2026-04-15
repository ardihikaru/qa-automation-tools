/**
 * Flow definitions for different roles
 */

export interface FlowStep {
  action: 'click' | 'type' | 'select' | 'wait' | 'navigate';
  selector: string;
  value?: string;
  wait?: number;
}

export interface FlowDefinition {
  id: string;
  name: string;
  role: 'admin' | 'doctor' | 'cashier';
  description: string;
  steps: FlowStep[];
}

// Admin flows
const adminFlows: FlowDefinition[] = [
  {
    id: 'admin-login',
    name: 'Admin Login',
    role: 'admin',
    description: 'Login as system administrator',
    steps: [
      { action: 'navigate', selector: '/login' },
      { action: 'type', selector: '#email', value: '{{username}}' },
      { action: 'type', selector: '#password', value: '{{password}}' },
      { action: 'click', selector: 'button[type="submit"]' },
      { action: 'wait', selector: '.dashboard', wait: 2000 },
    ],
  },
  {
    id: 'admin-create-user',
    name: 'Create User',
    role: 'admin',
    description: 'Create a new user in the system',
    steps: [
      { action: 'navigate', selector: '/users/new' },
      { action: 'type', selector: '#name', value: 'Test User' },
      { action: 'type', selector: '#email', value: 'test@example.com' },
      { action: 'type', selector: '#phone', value: '1234567890' },
      { action: 'select', selector: '#role', value: 'doctor' },
      { action: 'click', selector: 'button[type="submit"]' },
      { action: 'wait', selector: '.success-message', wait: 2000 },
    ],
  },
  {
    id: 'admin-view-users',
    name: 'View Users',
    role: 'admin',
    description: 'List and search users',
    steps: [
      { action: 'navigate', selector: '/users' },
      { action: 'type', selector: '#search', value: 'test' },
      { action: 'wait', selector: '.user-table', wait: 1000 },
    ],
  },
  {
    id: 'admin-settings',
    name: 'System Settings',
    role: 'admin',
    description: 'Configure system settings',
    steps: [
      { action: 'navigate', selector: '/settings' },
      { action: 'click', selector: '.tab-general' },
      { action: 'wait', selector: '.settings-form', wait: 500 },
    ],
  },
];

// Doctor flows
const doctorFlows: FlowDefinition[] = [
  {
    id: 'doctor-login',
    name: 'Doctor Login',
    role: 'doctor',
    description: 'Login as doctor',
    steps: [
      { action: 'navigate', selector: '/login' },
      { action: 'type', selector: '#email', value: '{{username}}' },
      { action: 'type', selector: '#password', value: '{{password}}' },
      { action: 'click', selector: 'button[type="submit"]' },
      { action: 'wait', selector: '.dashboard', wait: 2000 },
    ],
  },
  {
    id: 'doctor-view-patients',
    name: 'View Patients',
    role: 'doctor',
    description: 'View patient list',
    steps: [
      { action: 'navigate', selector: '/patients' },
      { action: 'wait', selector: '.patient-table', wait: 1000 },
    ],
  },
  {
    id: 'doctor-create-note',
    name: 'Create Medical Note',
    role: 'doctor',
    description: 'Create a medical note for a patient',
    steps: [
      { action: 'navigate', selector: '/patients/1/notes/new' },
      { action: 'type', selector: '#diagnosis', value: 'Common cold' },
      { action: 'type', selector: '#treatment', value: 'Rest and fluids' },
      { action: 'click', selector: 'button[type="submit"]' },
      { action: 'wait', selector: '.success-message', wait: 2000 },
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
    steps: [
      { action: 'navigate', selector: '/login' },
      { action: 'type', selector: '#email', value: '{{username}}' },
      { action: 'type', selector: '#password', value: '{{password}}' },
      { action: 'click', selector: 'button[type="submit"]' },
      { action: 'wait', selector: '.dashboard', wait: 2000 },
    ],
  },
  {
    id: 'cashier-billing',
    name: 'View Billing',
    role: 'cashier',
    description: 'View billing dashboard',
    steps: [
      { action: 'navigate', selector: '/billing' },
      { action: 'wait', selector: '.billing-table', wait: 1000 },
    ],
  },
  {
    id: 'cashier-create-invoice',
    name: 'Create Invoice',
    role: 'cashier',
    description: 'Create a new invoice',
    steps: [
      { action: 'navigate', selector: '/invoices/new' },
      { action: 'type', selector: '#patient-name', value: 'John Doe' },
      { action: 'type', selector: '#amount', value: '100000' },
      { action: 'click', selector: 'button[type="submit"]' },
      { action: 'wait', selector: '.success-message', wait: 2000 },
    ],
  },
];

// Export all flows
export function loadFlows(): FlowDefinition[] {
  return [...adminFlows, ...doctorFlows, ...cashierFlows];
}

export function getFlowsByRole(role: string): FlowDefinition[] {
  return loadFlows().filter(f => f.role === role);
}

export { FlowDefinition, FlowStep };