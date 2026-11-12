/**
 * Documentation flows per credential role, ported from playwright-docs
 * src/flows/flowLoader.ts. Login is not a flow step any more: the executor
 * logs in once per role (hisv3/login.ts), and no credential ever appears here.
 * Keys are the role names used in the credentials file (config.yaml `credentials.*`).
 */
export interface FlowStep {
  action: 'navigate' | 'click' | 'type' | 'select' | 'wait';
  selector?: string;
  value?: string;
  wait?: number;
}

export interface FlowDefinition {
  id: string;
  name: string;
  description: string;
  steps: FlowStep[];
}

const visit = (id: string, name: string, description: string, path: string, wait = 3000): FlowDefinition => ({
  id,
  name,
  description,
  steps: [
    { action: 'navigate', selector: path },
    { action: 'wait', wait },
  ],
});

const OUTPATIENT_DASHBOARD = '/administration/encounter/outpatient-dashboard';

export const FLOWS: Record<string, FlowDefinition[]> = {
  super_admin: [
    visit('super-admin-dashboard', 'Super Admin Dashboard', 'View super admin dashboard after login', OUTPATIENT_DASHBOARD),
    visit('super-admin-user-management', 'User Management', 'Access user management menu', '/administration/user'),
  ],
  admin: [visit('admin-dashboard', 'Admin Dashboard', 'View admin dashboard', OUTPATIENT_DASHBOARD, 2000)],
  doctor_gp: [
    visit('doctor-gp-dashboard', 'Doctor GP Dashboard', 'View doctor queue', '/doctor/queue'),
    visit('doctor-gp-patient-list', 'View Patient List', 'View list of patients', '/doctor/patient'),
    visit('doctor-gp-medical-record', 'Medical Record', 'Access medical record', '/doctor/medical-record'),
    visit('doctor-gp-prescription', 'Prescription', 'Create prescription', '/doctor/prescription'),
  ],
  nurse: [
    visit('nurse-dashboard', 'Nurse Dashboard', 'View nurse queue', '/nurse/queue'),
    visit('nurse-patient-vitals', 'Patient Vitals', 'Record patient vitals', '/nurse/vitals'),
  ],
  admission: [
    visit('admission-dashboard', 'Admission Dashboard', 'View admission dashboard', OUTPATIENT_DASHBOARD),
    visit('admission-new-patient', 'New Patient Registration', 'Register new patient', '/administration/patient/new'),
    visit('admission-queue', 'Queue Management', 'Manage patient queue', '/administration/encounter/queue'),
  ],
  cashier: [
    visit('cashier-dashboard', 'Cashier Dashboard', 'View cashier dashboard', '/cashier'),
    visit('cashier-billing', 'Billing', 'View billing transactions', '/cashier/billing'),
    visit('cashier-payment', 'Payment', 'Process payment', '/cashier/payment'),
  ],
  pharmacy: [
    visit('pharmacy-dashboard', 'Pharmacy Dashboard', 'View pharmacy dashboard', '/pharmacy'),
    visit('pharmacy-dispense', 'Dispense Medicine', 'Dispense medicine to patient', '/pharmacy/dispense'),
    visit('pharmacy-inventory', 'Inventory', 'View medicine inventory', '/pharmacy/inventory'),
  ],
  laboratory: [
    visit('laboratory-dashboard', 'Laboratory Dashboard', 'View laboratory dashboard', '/laboratory'),
    visit('laboratory-results', 'Lab Results', 'View lab results', '/laboratory/results'),
    visit('laboratory-specimen', 'Specimen Management', 'Manage specimens', '/laboratory/specimen'),
  ],
  radiographer: [visit('radiographer-dashboard', 'Radiographer Dashboard', 'View radiographer dashboard', '/radiology')],
};

/** Flows for a role; roles without curated flows get a landing-page flow. */
export function flowsForRole(role: string, only: string[] = []): FlowDefinition[] {
  const flows = FLOWS[role] ?? [visit(`${role}-landing`, `${role} landing page`, `Landing page after login as ${role}`, '/')];
  return only.length ? flows.filter((f) => only.includes(f.id)) : flows;
}
