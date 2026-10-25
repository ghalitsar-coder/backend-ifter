import { beforeAll, describe, expect, test } from 'e2e';

const API_URL = (process.env.API_URL ?? '').replace(/\/$/, '');
const ADMIN_USER = process.env.E2E_ADMIN_USER ?? 'e2e-admin';
const ADMIN_PASSWORD = process.env.E2E_ADMIN_PASSWORD ?? 'E2eAdminPass123!';
const PASSWORD = 'E2ePassword123!';
const ATTENDANCE_DATE = '2026-09-02';
const HOLIDAY_DATE = '2026-09-03';
const EXCEL = 'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet';

type RequestOptions = {
  method?: string;
  token?: string;
  body?: unknown;
  headers?: Record<string, string>;
};

type ApiResult = {
  res: Response;
  text: string;
  json: any;
};

type Session = {
  token: string;
  refreshCookie: string;
};

type Seed = {
  adminToken: string;
  yearID: string;
  term1ID: string;
  term2ID: string;
  classAID: string;
  classBID: string;
  classCID: string;
  teacher1UserID: string;
  teacher2UserID: string;
  principalUserID: string;
  extraUserID: string;
  teacher1ID: string;
  teacher2ID: string;
  teacherBID: string;
  student1ID: string;
  student2ID: string;
  student3ID: string;
  student4ID: string;
  student5ID: string;
  dutyID: string;
  teacherToken: string;
  dutyToken: string;
  principalToken: string;
};

let seed: Seed;

function requireConfig() {
  if (!API_URL) {
    throw new Error('API_URL is required, for example API_URL=http://127.0.0.1:8081');
  }
}

async function request(path: string, options: RequestOptions = {}): Promise<ApiResult> {
  const headers = new Headers(options.headers);
  if (options.token) {
    headers.set('authorization', `Bearer ${options.token}`);
  }
  let body: BodyInit | undefined;
  if (options.body !== undefined) {
    headers.set('content-type', 'application/json');
    body = JSON.stringify(options.body);
  }
  const res = await fetch(`${API_URL}${path}`, {
    method: options.method ?? 'GET',
    headers,
    body,
  });
  const text = await res.text();
  let json: any = null;
  if (text) {
    try {
      json = JSON.parse(text);
    } catch {
      json = null;
    }
  }
  return { res, text, json };
}

function expectStatus(result: ApiResult, expected: number, label: string) {
  if (result.res.status !== expected) {
    throw new Error(`${label}: expected HTTP ${expected}, got ${result.res.status}: ${result.text.slice(0, 500)}`);
  }
}

function refreshCookie(res: Response): string {
  const header = (res.headers as Headers & { getSetCookie?: () => string[] }).getSetCookie?.()?.[0]
    ?? res.headers.get('set-cookie')
    ?? '';
  const match = header.match(/ifter_refresh=([^;]+)/);
  return match ? `ifter_refresh=${match[1]}` : '';
}

async function login(loginName: string, password: string): Promise<Session> {
  const result = await request('/api/v1/auth/login', {
    method: 'POST',
    body: { login: loginName, password },
  });
  expectStatus(result, 200, `login ${loginName}`);
  return {
    token: result.json.access_token,
    refreshCookie: refreshCookie(result.res),
  };
}

async function createUser(username: string, fullName: string, roles: string[]) {
  const result = await request('/api/v1/users', {
    method: 'POST',
    token: seed.adminToken,
    body: {
      username,
      full_name: fullName,
      email: `${username}@example.test`,
      roles,
    },
  });
  expectStatus(result, 201, `create user ${username}`);
  expect(result.json.user.status).toBe('pending');
  expect(result.json.credential.token).toBeTruthy();
  return {
    id: result.json.user.id as string,
    credentialToken: result.json.credential.token as string,
  };
}

async function activateUser(username: string, credentialToken: string, password: string) {
  const result = await request('/api/v1/auth/activate', {
    method: 'POST',
    body: { token: credentialToken, password },
  });
  expectStatus(result, 204, `activate ${username}`);
}

async function createActivatedUser(username: string, fullName: string, roles: string[]) {
  const user = await createUser(username, fullName, roles);
  await activateUser(username, user.credentialToken, PASSWORD);
  const session = await login(username, PASSWORD);
  return { id: user.id, token: session.token };
}

async function createTeacher(employeeNumber: string, fullName: string, userID?: string) {
  const result = await request('/api/v1/teachers', {
    method: 'POST',
    token: seed.adminToken,
    body: {
      employee_number: employeeNumber,
      full_name: fullName,
      email: `${employeeNumber.toLowerCase()}@example.test`,
      user_id: userID ?? '',
    },
  });
  expectStatus(result, 201, `create teacher ${employeeNumber}`);
  return result.json.teacher.id as string;
}

async function createStudent(
  studentNumber: string,
  fullName: string,
  gender: 'L' | 'P',
  classID?: string,
) {
  const result = await request('/api/v1/students', {
    method: 'POST',
    token: seed.adminToken,
    body: {
      student_number: studentNumber,
      full_name: fullName,
      gender,
      admission_year: 2026,
      class_id: classID ?? '',
      class_start_date: classID ? '2026-07-01' : '',
    },
  });
  expectStatus(result, 201, `create student ${studentNumber}`);
  return result.json.student.id as string;
}

async function xlsx(path: string, token: string) {
  const res = await fetch(`${API_URL}${path}`, {
    headers: { authorization: `Bearer ${token}` },
  });
  const bytes = new Uint8Array(await res.arrayBuffer());
  return { res, bytes };
}

function expectXlsx(download: { res: Response; bytes: Uint8Array }, label: string) {
  if (download.res.status !== 200) {
    throw new Error(`${label}: expected HTTP 200, got ${download.res.status}`);
  }
  expect(download.res.headers.get('content-type')).toContain(EXCEL);
  expect(download.bytes.length).toBeGreaterThan(100);
  expect(download.bytes[0]).toBe(0x50);
  expect(download.bytes[1]).toBe(0x4b);
}

async function uploadTemplate(path: string, template: Uint8Array) {
  const form = new FormData();
  form.set('mode', 'create_only');
  form.set('file', new Blob([new Uint8Array(template)], { type: EXCEL }), 'import.xlsx');
  const res = await fetch(`${API_URL}${path}`, {
    method: 'POST',
    headers: { authorization: `Bearer ${seed.adminToken}` },
    body: form,
  });
  const text = await res.text();
  let json: any = null;
  try {
    json = text ? JSON.parse(text) : null;
  } catch {
    json = null;
  }
  return { res, text, json };
}

// ---------------------------------------------------------------------------
// Wave-2 helpers (dispensations multipart, presence timestamps)
// ---------------------------------------------------------------------------

const ROTATION_WED_1 = '2026-09-02';
const ROTATION_WED_2 = '2026-09-09';
const UNKNOWN_TEACHER_UUID = 'f7a9c214-2c3b-4a1e-9d2f-000000000e20';
const SCHOOL_LAT = -6.2;
const SCHOOL_LNG = 106.816666;

function pdfDocumentBytes(): Uint8Array {
  return new TextEncoder().encode('%PDF-1.4\n1 0 obj <<>> endobj\ntrailer <<>>\n%%EOF\n');
}

async function createDispensation(
  token: string,
  fields: Record<string, string>,
  document?: { filename: string; contentType: string; bytes: Uint8Array },
): Promise<ApiResult> {
  if (!document) {
    return request('/api/v1/dispensations', { method: 'POST', token, body: fields });
  }
  const form = new FormData();
  for (const [key, value] of Object.entries(fields)) {
    form.set(key, value);
  }
  form.set('document', new Blob([document.bytes], { type: document.contentType }), document.filename);
  const res = await fetch(`${API_URL}/api/v1/dispensations`, {
    method: 'POST',
    headers: { authorization: `Bearer ${token}` },
    body: form,
  });
  const text = await res.text();
  let json: any = null;
  try {
    json = text ? JSON.parse(text) : null;
  } catch {
    json = null;
  }
  return { res, text, json };
}

async function downloadBinary(path: string, token: string) {
  const res = await fetch(`${API_URL}${path}`, {
    headers: { authorization: `Bearer ${token}` },
  });
  const bytes = new Uint8Array(await res.arrayBuffer());
  return { res, bytes };
}

describe('backend-ifter API', { serial: true, timeout: 300_000 }, () => {
  beforeAll(async () => {
    requireConfig();
    const admin = await login(ADMIN_USER, ADMIN_PASSWORD);
    seed = {
      adminToken: admin.token,
    } as Seed;

    const year = await request('/api/v1/academic-years', {
      method: 'POST',
      token: seed.adminToken,
      body: {
        name: '2026/2027 E2E',
        start_date: '2026-07-01',
        end_date: '2027-06-30',
        is_active: true,
      },
    });
    expectStatus(year, 201, 'create academic year');
    seed.yearID = year.json.academic_year.id;

    const term1 = await request(`/api/v1/academic-years/${seed.yearID}/terms`, {
      method: 'POST',
      token: seed.adminToken,
      body: {
        term_number: 1,
        name: 'Semester 1 E2E',
        start_date: '2026-07-01',
        end_date: '2026-12-31',
      },
    });
    expectStatus(term1, 201, 'create term 1');
    seed.term1ID = term1.json.term.id;

    const term2 = await request(`/api/v1/academic-years/${seed.yearID}/terms`, {
      method: 'POST',
      token: seed.adminToken,
      body: {
        term_number: 2,
        name: 'Semester 2 E2E',
        start_date: '2027-01-01',
        end_date: '2027-06-30',
      },
    });
    expectStatus(term2, 201, 'create term 2');
    seed.term2ID = term2.json.term.id;

    const classA = await request('/api/v1/classes', {
      method: 'POST',
      token: seed.adminToken,
      body: {
        academic_year_id: seed.yearID,
        name: 'X-A E2E',
        grade_level: 10,
        major: 'IPA',
      },
    });
    expectStatus(classA, 201, 'create class A');
    seed.classAID = classA.json.class.id;

    const classB = await request('/api/v1/classes', {
      method: 'POST',
      token: seed.adminToken,
      body: {
        academic_year_id: seed.yearID,
        name: 'X-B E2E',
        grade_level: 10,
        major: 'IPS',
      },
    });
    expectStatus(classB, 201, 'create class B');
    seed.classBID = classB.json.class.id;

    const classC = await request('/api/v1/classes', {
      method: 'POST',
      token: seed.adminToken,
      body: {
        academic_year_id: seed.yearID,
        name: 'X-C E2E',
        grade_level: 10,
      },
    });
    expectStatus(classC, 201, 'create class C');
    seed.classCID = classC.json.class.id;

    const teacher1 = await createActivatedUser('e2e.teacher', 'E2E Teacher', ['teacher']);
    const teacher2 = await createActivatedUser('e2e.duty', 'E2E Duty Teacher', ['duty_teacher']);
    const principal = await createActivatedUser('e2e.principal', 'E2E Principal', ['principal']);
    const extra = await createActivatedUser('e2e.extra', 'E2E Extra User', ['teacher']);
    seed.teacher1UserID = teacher1.id;
    seed.teacher2UserID = teacher2.id;
    seed.principalUserID = principal.id;
    seed.extraUserID = extra.id;
    seed.teacherToken = teacher1.token;
    seed.dutyToken = teacher2.token;
    seed.principalToken = principal.token;

    seed.teacher1ID = await createTeacher('E2E-T1', 'E2E Teacher', seed.teacher1UserID);
    seed.teacher2ID = await createTeacher('E2E-T2', 'E2E Duty Teacher', seed.teacher2UserID);
    seed.teacherBID = await createTeacher('E2E-T3', 'E2E Substitute Teacher');

    const homeroom = await request(`/api/v1/classes/${seed.classAID}/homeroom`, {
      method: 'PUT',
      token: seed.adminToken,
      body: { teacher_id: seed.teacher1ID, start_date: '2026-07-01' },
    });
    expectStatus(homeroom, 200, 'assign homeroom');

    const duty = await request('/api/v1/duty-assignments', {
      method: 'POST',
      token: seed.adminToken,
      body: {
        teacher_id: seed.teacher2ID,
        weekday: 3,
        start_date: '2026-07-01',
      },
    });
    expectStatus(duty, 201, 'create duty assignment');
    seed.dutyID = duty.json.id;

    seed.student1ID = await createStudent('E2E-001', 'Siswa Satu', 'L', seed.classAID);
    seed.student2ID = await createStudent('E2E-002', 'Siswa Dua', 'P', seed.classAID);
    seed.student3ID = await createStudent('E2E-003', 'Siswa Tiga', 'L', seed.classAID);
    seed.student4ID = await createStudent('E2E-004', 'Siswa Empat', 'P', seed.classBID);
    seed.student5ID = await createStudent('E2E-005', 'Siswa Lima', 'L');
  });

  test('health, readiness, and metrics endpoints respond', async () => {
    const health = await request('/healthz');
    expectStatus(health, 200, 'health');
    expect(health.json.status).toBe('ok');

    const ready = await request('/readyz');
    expectStatus(ready, 200, 'ready');
    expect(ready.json.status).toBe('ok');

    const metrics = await request('/metrics');
    expectStatus(metrics, 200, 'metrics');
    expect(metrics.text).toContain('ifter_db_pool_connections');
    expect(metrics.text).toContain('go_goroutines');
  });

  test('auth login, session, refresh, logout, and RBAC work', async () => {
    const invalid = await request('/api/v1/auth/login', {
      method: 'POST',
      body: { login: ADMIN_USER, password: 'wrong-password' },
    });
    expectStatus(invalid, 401, 'invalid login');
    expect(invalid.json.code).toBe('invalid_credentials');

    const blockedOrigin = await request('/api/v1/auth/login', {
      method: 'POST',
      headers: { origin: 'https://evil.example.test' },
      body: { login: ADMIN_USER, password: ADMIN_PASSWORD },
    });
    expectStatus(blockedOrigin, 403, 'blocked origin');

    const admin = await login(ADMIN_USER, ADMIN_PASSWORD);
    expect(admin.refreshCookie).toContain('ifter_refresh=');

    const me = await request('/api/v1/auth/me', { token: admin.token });
    expectStatus(me, 200, 'me');
    expect(me.json.user.username).toBe(ADMIN_USER);
    expect(me.json.user.roles).toContain('admin');

    const anonymous = await request('/api/v1/auth/me');
    expectStatus(anonymous, 401, 'anonymous me');

    const noCookie = await request('/api/v1/auth/refresh', { method: 'POST' });
    expectStatus(noCookie, 401, 'refresh without cookie');

    const refreshed = await request('/api/v1/auth/refresh', {
      method: 'POST',
      headers: { cookie: admin.refreshCookie },
    });
    expectStatus(refreshed, 200, 'refresh');
    expect(refreshed.json.access_token).toBeTruthy();
    expect(refreshed.json.user.username).toBe(ADMIN_USER);
    const rotatedCookie = refreshCookie(refreshed.res);
    expect(rotatedCookie).toContain('ifter_refresh=');

    const logout = await request('/api/v1/auth/logout', {
      method: 'POST',
      headers: { cookie: rotatedCookie },
    });
    expectStatus(logout, 204, 'logout');

    const afterLogout = await request('/api/v1/auth/refresh', {
      method: 'POST',
      headers: { cookie: rotatedCookie },
    });
    expectStatus(afterLogout, 401, 'refresh after logout');
  });

  test('users, roles, and admin-only access work', async () => {
    const users = await request('/api/v1/users?limit=100', { token: seed.adminToken });
    expectStatus(users, 200, 'list users');
    expect(users.json.total).toBeGreaterThanOrEqual(5);
    expect(users.json.data.map((user: any) => user.username)).toContain('e2e.teacher');

    const roles = await request('/api/v1/roles', { token: seed.adminToken });
    expectStatus(roles, 200, 'list roles');
    expect(roles.json.data.map((role: any) => role.code)).toEqual(
      expect.arrayContaining(['admin', 'teacher', 'duty_teacher', 'principal']),
    );

    const detail = await request(`/api/v1/users/${seed.extraUserID}`, { token: seed.adminToken });
    expectStatus(detail, 200, 'get user');
    expect(detail.json.user.id).toBe(seed.extraUserID);

    const updateRoles = await request(`/api/v1/users/${seed.extraUserID}/roles`, {
      method: 'PUT',
      token: seed.adminToken,
      body: { roles: ['duty_teacher', 'principal'] },
    });
    expectStatus(updateRoles, 200, 'update roles');
    expect(updateRoles.json.user.roles).toEqual(expect.arrayContaining(['duty_teacher', 'principal']));

    const inactive = await request(`/api/v1/users/${seed.extraUserID}/status`, {
      method: 'PATCH',
      token: seed.adminToken,
      body: { status: 'inactive' },
    });
    expectStatus(inactive, 200, 'deactivate user');
    expect(inactive.json.user.status).toBe('inactive');

    const credential = await request(`/api/v1/users/${seed.extraUserID}/credential-reset`, {
      method: 'POST',
      token: seed.adminToken,
    });
    expectStatus(credential, 200, 'credential reset');
    expect(credential.json.credential.purpose).toBe('reset');
    expect(credential.json.credential.token).toBeTruthy();

    const teacherForbidden = await request('/api/v1/users', { token: seed.teacherToken });
    expectStatus(teacherForbidden, 403, 'teacher list users');
    expect(teacherForbidden.json.code).toBe('forbidden');

    const anonymous = await request('/api/v1/users');
    expectStatus(anonymous, 401, 'anonymous list users');
  });

  test('academic years and terms endpoints work', async () => {
    const list = await request('/api/v1/academic-years', { token: seed.adminToken });
    expectStatus(list, 200, 'list academic years');
    expect(list.json.data.map((year: any) => year.id)).toContain(seed.yearID);

    const detail = await request(`/api/v1/academic-years/${seed.yearID}`, { token: seed.adminToken });
    expectStatus(detail, 200, 'get academic year');
    expect(detail.json.academic_year.is_active).toBe(true);

    const update = await request(`/api/v1/academic-years/${seed.yearID}`, {
      method: 'PUT',
      token: seed.adminToken,
      body: {
        name: '2026/2027 E2E Updated',
        start_date: '2026-07-01',
        end_date: '2027-06-30',
        is_active: true,
      },
    });
    expectStatus(update, 200, 'update academic year');
    expect(update.json.academic_year.name).toBe('2026/2027 E2E Updated');

    const terms = await request(`/api/v1/academic-years/${seed.yearID}/terms`, {
      token: seed.adminToken,
    });
    expectStatus(terms, 200, 'list terms');
    expect(terms.json.data).toHaveLength(2);

    const term = await request(`/api/v1/terms/${seed.term2ID}`, { token: seed.adminToken });
    expectStatus(term, 200, 'get term');
    expect(term.json.term.term_number).toBe(2);

    const updateTerm = await request(`/api/v1/terms/${seed.term2ID}`, {
      method: 'PUT',
      token: seed.adminToken,
      body: {
        term_number: 2,
        name: 'Semester 2 E2E Updated',
        start_date: '2027-01-01',
        end_date: '2027-06-30',
      },
    });
    expectStatus(updateTerm, 200, 'update term');
    expect(updateTerm.json.term.name).toBe('Semester 2 E2E Updated');

    const invalidTerm = await request(`/api/v1/academic-years/${seed.yearID}/terms`, {
      method: 'POST',
      token: seed.adminToken,
      body: {
        term_number: 9,
        name: 'Invalid',
        start_date: '2027-01-01',
        end_date: '2027-06-30',
      },
    });
    expectStatus(invalidTerm, 400, 'invalid term');
  });

  test('classes, homeroom, and class students work', async () => {
    const list = await request(`/api/v1/classes?academic_year_id=${seed.yearID}&limit=100`, {
      token: seed.adminToken,
    });
    expectStatus(list, 200, 'list classes');
    expect(list.json.total).toBeGreaterThanOrEqual(3);

    const detail = await request(`/api/v1/classes/${seed.classAID}`, { token: seed.adminToken });
    expectStatus(detail, 200, 'get class');
    expect(detail.json.class.name).toBe('X-A E2E');
    expect(detail.json.class.homeroom_teacher_id).toBe(seed.teacher1ID);

    const students = await request(`/api/v1/classes/${seed.classAID}/students`, {
      token: seed.adminToken,
    });
    expectStatus(students, 200, 'class students');
    expect(students.json.data).toHaveLength(3);

    const update = await request(`/api/v1/classes/${seed.classCID}`, {
      method: 'PUT',
      token: seed.adminToken,
      body: { name: 'X-C E2E Updated', grade_level: 10, major: 'Bahasa' },
    });
    expectStatus(update, 200, 'update class');
    expect(update.json.class.name).toBe('X-C E2E Updated');

    const archived = await request(`/api/v1/classes/${seed.classCID}/status`, {
      method: 'PATCH',
      token: seed.adminToken,
      body: { status: 'archived' },
    });
    expectStatus(archived, 200, 'archive class');
    expect(archived.json.class.status).toBe('archived');

    const filtered = await request(`/api/v1/classes?academic_year_id=${seed.yearID}&status=archived`, {
      token: seed.adminToken,
    });
    expectStatus(filtered, 200, 'list archived classes');
    expect(filtered.json.data.map((row: any) => row.id)).toContain(seed.classCID);
  });

  test('teachers and teacher status work', async () => {
    const list = await request('/api/v1/teachers?search=E2E&limit=100', {
      token: seed.adminToken,
    });
    expectStatus(list, 200, 'list teachers');
    expect(list.json.total).toBeGreaterThanOrEqual(3);

    const detail = await request(`/api/v1/teachers/${seed.teacher1ID}`, { token: seed.adminToken });
    expectStatus(detail, 200, 'get teacher');
    expect(detail.json.teacher.user_id).toBe(seed.teacher1UserID);

    const update = await request(`/api/v1/teachers/${seed.teacherBID}`, {
      method: 'PUT',
      token: seed.adminToken,
      body: {
        employee_number: 'E2E-T3',
        full_name: 'E2E Substitute Teacher Updated',
        email: 'e2e-t3@example.test',
      },
    });
    expectStatus(update, 200, 'update teacher');
    expect(update.json.teacher.full_name).toBe('E2E Substitute Teacher Updated');

    const inactive = await request(`/api/v1/teachers/${seed.teacherBID}/status`, {
      method: 'PATCH',
      token: seed.adminToken,
      body: { status: 'inactive' },
    });
    expectStatus(inactive, 200, 'deactivate teacher');
    expect(inactive.json.teacher.status).toBe('inactive');

    const active = await request(`/api/v1/teachers/${seed.teacherBID}/status`, {
      method: 'PATCH',
      token: seed.adminToken,
      body: { status: 'active' },
    });
    expectStatus(active, 200, 'reactivate teacher');
    expect(active.json.teacher.status).toBe('active');

    const forbidden = await request('/api/v1/teachers', { token: seed.teacherToken });
    expectStatus(forbidden, 403, 'teacher list teachers');
  });

  test('students, enrollment changes, and student status work', async () => {
    const list = await request(`/api/v1/students?class_id=${seed.classAID}&limit=100`, {
      token: seed.adminToken,
    });
    expectStatus(list, 200, 'list students');
    expect(list.json.total).toBe(3);

    const detail = await request(`/api/v1/students/${seed.student1ID}`, { token: seed.adminToken });
    expectStatus(detail, 200, 'get student');
    expect(detail.json.student.current_class.id).toBe(seed.classAID);
    expect(detail.json.student.enrollments).toHaveLength(1);

    const update = await request(`/api/v1/students/${seed.student5ID}`, {
      method: 'PUT',
      token: seed.adminToken,
      body: {
        student_number: 'E2E-005',
        full_name: 'Siswa Lima Updated',
        gender: 'L',
        admission_year: 2026,
        guardian_name: 'Wali E2E',
        guardian_phone: '08123456789',
      },
    });
    expectStatus(update, 200, 'update student');
    expect(update.json.student.full_name).toBe('Siswa Lima Updated');

    const assigned = await request(`/api/v1/students/${seed.student5ID}/class`, {
      method: 'PUT',
      token: seed.adminToken,
      body: { class_id: seed.classBID, start_date: '2026-07-01' },
    });
    expectStatus(assigned, 200, 'assign student class');
    expect(assigned.json.student.current_class.id).toBe(seed.classBID);
    expect(assigned.json.student.enrollments).toHaveLength(1);

    const inactive = await request(`/api/v1/students/${seed.student5ID}/status`, {
      method: 'PATCH',
      token: seed.adminToken,
      body: { status: 'inactive' },
    });
    expectStatus(inactive, 200, 'deactivate student');
    expect(inactive.json.student.status).toBe('inactive');

    const filtered = await request(`/api/v1/students?class_id=${seed.classBID}&limit=100`, {
      token: seed.adminToken,
    });
    expectStatus(filtered, 200, 'list class B students');
    expect(filtered.json.total).toBe(2);
  });

  test('attendance settings, holidays, and duty assignments work', async () => {
    const settings = await request('/api/v1/attendance/settings', { token: seed.teacherToken });
    expectStatus(settings, 200, 'attendance settings');
    expect(settings.json.timezone).toBe('Asia/Jakarta');
    expect(settings.json.schedules).toHaveLength(5);

    const update = await request('/api/v1/attendance/settings', {
      method: 'PUT',
      token: seed.adminToken,
      body: {
        school_name: settings.json.school_name,
        timezone: settings.json.timezone,
        attendance_auto_close_time: settings.json.attendance_auto_close_time,
        schedules: settings.json.schedules.map((row: any) => ({
          weekday: row.weekday,
          start_time: row.start_time,
          late_tolerance_minutes: row.late_tolerance_minutes,
        })),
      },
    });
    expectStatus(update, 200, 'update attendance settings');
    expect(update.json.schedules).toHaveLength(5);

    const holiday = await request('/api/v1/attendance/holidays', {
      method: 'POST',
      token: seed.adminToken,
      body: { holiday_date: HOLIDAY_DATE, name: 'Hari Libur E2E' },
    });
    expectStatus(holiday, 201, 'create holiday');
    expect(holiday.json.holiday_date).toBe(HOLIDAY_DATE);

    const holidays = await request(`/api/v1/attendance/holidays?from=${HOLIDAY_DATE}&to=${HOLIDAY_DATE}`, {
      token: seed.teacherToken,
    });
    expectStatus(holidays, 200, 'list holidays');
    expect(holidays.json.data).toHaveLength(1);

    const dutyList = await request('/api/v1/duty-assignments?weekday=3', {
      token: seed.adminToken,
    });
    expectStatus(dutyList, 200, 'list duty assignments');
    expect(dutyList.json.data.map((row: any) => row.id)).toContain(seed.dutyID);

    const endDuty = await request(`/api/v1/duty-assignments/${seed.dutyID}/end`, {
      method: 'PUT',
      token: seed.adminToken,
      body: { end_date: '2027-06-30' },
    });
    expectStatus(endDuty, 200, 'end duty assignment');
    expect(endDuty.json.end_date).toBe('2027-06-30');

    const forbidden = await request('/api/v1/attendance/settings', { token: seed.dutyToken });
    expectStatus(forbidden, 200, 'duty read settings');
  });

  test('attendance roster, save, close, corrections, and reports work', async () => {
    const roster = await request(
      `/api/v1/attendance/roster?class_id=${seed.classAID}&date=${ATTENDANCE_DATE}`,
      { token: seed.adminToken },
    );
    expectStatus(roster, 200, 'attendance roster');
    expect(roster.json.summary.total).toBe(3);
    expect(roster.json.summary.unrecorded).toBe(3);
    expect(roster.json.effective).toBe(true);

    const teacherRoster = await request(
      `/api/v1/attendance/roster?class_id=${seed.classAID}&date=${ATTENDANCE_DATE}`,
      { token: seed.teacherToken },
    );
    expectStatus(teacherRoster, 200, 'homeroom teacher roster');

    const teacherForbidden = await request(
      `/api/v1/attendance/roster?class_id=${seed.classBID}&date=${ATTENDANCE_DATE}`,
      { token: seed.teacherToken },
    );
    expectStatus(teacherForbidden, 403, 'teacher other class roster');

    const principalRoster = await request(
      `/api/v1/attendance/roster?class_id=${seed.classBID}&date=${ATTENDANCE_DATE}`,
      { token: seed.principalToken },
    );
    // Wave-2 RBAC: roster is the recording screen and lives on AttendanceOps
    // (admin+teacher+duty_teacher); principal is read-only and must not reach it.
    // See docs/API_ADDENDUM.md (RBAC drift notes).
    expectStatus(principalRoster, 403, 'principal roster');
    expect(principalRoster.json.code).toBe('forbidden');

    const principalHistory = await request(
      `/api/v1/attendance/history?from=${ATTENDANCE_DATE}&to=${ATTENDANCE_DATE}&class_id=${seed.classBID}`,
      { token: seed.principalToken },
    );
    expectStatus(principalHistory, 200, 'principal attendance history');

    const dutyRoster = await request(
      `/api/v1/attendance/roster?class_id=${seed.classAID}&date=${ATTENDANCE_DATE}`,
      { token: seed.dutyToken },
    );
    expectStatus(dutyRoster, 200, 'duty teacher roster');

    const holidayRoster = await request(
      `/api/v1/attendance/roster?class_id=${seed.classAID}&date=${HOLIDAY_DATE}`,
      { token: seed.adminToken },
    );
    expectStatus(holidayRoster, 200, 'holiday roster');
    expect(holidayRoster.json.effective).toBe(false);

    const holidaySave = await request('/api/v1/attendance/bulk', {
      method: 'POST',
      token: seed.adminToken,
      body: {
        class_id: seed.classAID,
        date: HOLIDAY_DATE,
        records: [{ student_id: seed.student1ID, status: 'present' }],
      },
    });
    expectStatus(holidaySave, 400, 'save on holiday');

    const save = await request('/api/v1/attendance/bulk', {
      method: 'POST',
      token: seed.adminToken,
      body: {
        class_id: seed.classAID,
        date: ATTENDANCE_DATE,
        records: [
          { student_id: seed.student1ID, status: 'present' },
          { student_id: seed.student2ID, status: 'late', check_in_at: '07:45' },
          { student_id: seed.student3ID, status: 'absent' },
        ],
      },
    });
    expectStatus(save, 200, 'save attendance');
    expect(save.json.summary.recorded).toBe(3);
    expect(save.json.summary.present).toBe(1);
    expect(save.json.summary.late).toBe(1);
    expect(save.json.summary.absent).toBe(1);

    const student1 = save.json.students.find((row: any) => row.student_id === seed.student1ID);
    expect(student1.record.id).toBeTruthy();
    const recordID = student1.record.id as string;

    const record = await request(`/api/v1/attendance/records/${recordID}`, {
      token: seed.adminToken,
    });
    expectStatus(record, 200, 'get attendance record');
    expect(record.json.status).toBe('present');

    const correctionBeforeClose = await request(`/api/v1/attendance/records/${recordID}/corrections`, {
      method: 'POST',
      token: seed.adminToken,
      body: { status: 'late', reason: 'belum ditutup' },
    });
    expectStatus(correctionBeforeClose, 409, 'correction before close');

    const close = await request('/api/v1/attendance/close', {
      method: 'POST',
      token: seed.adminToken,
      body: { class_id: seed.classAID, date: ATTENDANCE_DATE },
    });
    expectStatus(close, 200, 'close attendance');
    expect(close.json.closure.close_type).toBe('manual');
    expect(close.json.summary.recorded).toBe(3);

    const closeAgain = await request('/api/v1/attendance/close', {
      method: 'POST',
      token: seed.adminToken,
      body: { class_id: seed.classAID, date: ATTENDANCE_DATE },
    });
    expectStatus(closeAgain, 409, 'close attendance twice');

    const correction = await request(`/api/v1/attendance/records/${recordID}/corrections`, {
      method: 'POST',
      token: seed.adminToken,
      body: {
        status: 'late',
        check_in_at: '08:00',
        notes: 'koreksi e2e',
        reason: 'kesalahan input',
      },
    });
    expectStatus(correction, 200, 'correct attendance');
    expect(correction.json.status).toBe('late');
    expect(correction.json.corrections).toHaveLength(1);

    const closedRoster = await request(
      `/api/v1/attendance/roster?class_id=${seed.classAID}&date=${ATTENDANCE_DATE}`,
      { token: seed.adminToken },
    );
    expectStatus(closedRoster, 200, 'closed roster');
    expect(closedRoster.json.closure.close_type).toBe('manual');
    expect(closedRoster.json.summary.present).toBe(0);
    expect(closedRoster.json.summary.late).toBe(2);
    expect(closedRoster.json.summary.absent).toBe(1);

    const history = await request(
      `/api/v1/attendance/history?from=${ATTENDANCE_DATE}&to=${ATTENDANCE_DATE}&class_id=${seed.classAID}`,
      { token: seed.adminToken },
    );
    expectStatus(history, 200, 'attendance history');
    expect(history.json.total).toBe(3);

    const summary = await request(
      `/api/v1/attendance/summary?from=${ATTENDANCE_DATE}&to=${ATTENDANCE_DATE}&class_id=${seed.classAID}`,
      { token: seed.adminToken },
    );
    expectStatus(summary, 200, 'attendance summary');
    expect(summary.json.data).toHaveLength(3);
    expect(summary.json.totals.present).toBe(0);
    expect(summary.json.totals.late).toBe(2);
    expect(summary.json.totals.absent).toBe(1);

    const report = await request(
      `/api/v1/reports/attendance?from=${ATTENDANCE_DATE}&to=${ATTENDANCE_DATE}&group_by=day&class_id=${seed.classAID}`,
      { token: seed.adminToken },
    );
    expectStatus(report, 200, 'attendance report');
    expect(report.json.data.length).toBeGreaterThanOrEqual(1);
    expect(report.json.totals.late).toBe(2);

    expectXlsx(
      await xlsx(
        `/api/v1/exports/attendance?from=${ATTENDANCE_DATE}&to=${ATTENDANCE_DATE}&class_id=${seed.classAID}`,
        seed.adminToken,
      ),
      'attendance export',
    );
    expectXlsx(
      await xlsx(
        `/api/v1/exports/attendance-report?from=${ATTENDANCE_DATE}&to=${ATTENDANCE_DATE}&group_by=day&class_id=${seed.classAID}`,
        seed.adminToken,
      ),
      'attendance report export',
    );

    const deleteHoliday = await request(`/api/v1/attendance/holidays/${HOLIDAY_DATE}`, {
      method: 'DELETE',
      token: seed.adminToken,
    });
    expectStatus(deleteHoliday, 204, 'delete holiday');
  });

  test('discipline categories, records, followups, points, and reports work', async () => {
    const categories = await request('/api/v1/discipline/categories?include_inactive=true', {
      token: seed.adminToken,
    });
    expectStatus(categories, 200, 'list discipline categories');
    const lateCategory = categories.json.data.find((row: any) => row.code === 'late');
    expect(lateCategory.id).toBeTruthy();

    const created = await request('/api/v1/discipline/categories', {
      method: 'POST',
      token: seed.adminToken,
      body: {
        code: 'e2e_uniform',
        name: 'Seragam E2E',
        description: 'Kategori uji e2e',
        default_points: 5,
      },
    });
    expectStatus(created, 201, 'create discipline category');
    const categoryID = created.json.id as string;
    expect(created.json.default_points).toBe(5);

    const updated = await request(`/api/v1/discipline/categories/${categoryID}`, {
      method: 'PUT',
      token: seed.adminToken,
      body: {
        code: 'e2e_uniform',
        name: 'Seragam E2E Updated',
        description: 'Kategori uji e2e',
        default_points: 6,
      },
    });
    expectStatus(updated, 200, 'update discipline category');
    expect(updated.json.default_points).toBe(6);

    const inactive = await request(`/api/v1/discipline/categories/${categoryID}/status`, {
      method: 'PATCH',
      token: seed.adminToken,
      body: { is_active: false },
    });
    expectStatus(inactive, 200, 'deactivate discipline category');
    expect(inactive.json.is_active).toBe(false);

    const inactiveRecord = await request('/api/v1/discipline/records', {
      method: 'POST',
      token: seed.adminToken,
      body: {
        student_id: seed.student1ID,
        category_id: categoryID,
        occurred_at: `${ATTENDANCE_DATE}T09:00:00+07:00`,
        description: 'Kategori nonaktif',
      },
    });
    expectStatus(inactiveRecord, 400, 'record with inactive category');

    const active = await request(`/api/v1/discipline/categories/${categoryID}/status`, {
      method: 'PATCH',
      token: seed.adminToken,
      body: { is_active: true },
    });
    expectStatus(active, 200, 'reactivate discipline category');
    expect(active.json.is_active).toBe(true);

    const autoCategoryRecord = await request('/api/v1/discipline/records', {
      method: 'POST',
      token: seed.adminToken,
      body: {
        student_id: seed.student1ID,
        category_id: lateCategory.id,
        occurred_at: `${ATTENDANCE_DATE}T09:00:00+07:00`,
        description: 'Kategori otomatis',
      },
    });
    expectStatus(autoCategoryRecord, 400, 'record with automatic category');

    const record = await request('/api/v1/discipline/records', {
      method: 'POST',
      token: seed.adminToken,
      body: {
        student_id: seed.student1ID,
        category_id: categoryID,
        occurred_at: `${ATTENDANCE_DATE}T09:00:00+07:00`,
        description: 'Seragam tidak lengkap',
      },
    });
    expectStatus(record, 201, 'create discipline record');
    expect(record.json.student_id).toBe(seed.student1ID);
    expect(record.json.points).toBe(6);
    const recordID = record.json.id as string;

    const list = await request(
      `/api/v1/discipline/records?from=2026-09-01&to=2026-09-30&student_id=${seed.student1ID}&limit=100`,
      { token: seed.adminToken },
    );
    expectStatus(list, 200, 'list discipline records');
    expect(list.json.data.map((row: any) => row.id)).toContain(recordID);

    const detail = await request(`/api/v1/discipline/records/${recordID}`, {
      token: seed.adminToken,
    });
    expectStatus(detail, 200, 'get discipline record');
    expect(detail.json.followups).toHaveLength(0);

    const followup = await request(`/api/v1/discipline/records/${recordID}/followups`, {
      method: 'POST',
      token: seed.adminToken,
      body: { status: 'under_review', notes: 'Ditinjau wali kelas' },
    });
    expectStatus(followup, 200, 'add discipline followup');
    expect(followup.json.followups).toHaveLength(1);
    expect(followup.json.followups[0].status).toBe('under_review');

    const points = await request(`/api/v1/discipline/records/${recordID}/points`, {
      method: 'PATCH',
      token: seed.adminToken,
      body: { points: 8, reason: 'penyesuaian e2e' },
    });
    expectStatus(points, 200, 'update discipline points');
    expect(points.json.points).toBe(8);

    const studentSummary = await request(`/api/v1/discipline/students/${seed.student1ID}`, {
      token: seed.adminToken,
    });
    expectStatus(studentSummary, 200, 'student discipline summary');
    expect(studentSummary.json.total_records).toBeGreaterThanOrEqual(1);
    expect(studentSummary.json.total_points).toBeGreaterThanOrEqual(8);
    expect(studentSummary.json.records.map((row: any) => row.id)).toContain(recordID);

    const report = await request(
      `/api/v1/reports/discipline?from=2026-09-01&to=2026-09-30&class_id=${seed.classAID}`,
      { token: seed.adminToken },
    );
    expectStatus(report, 200, 'discipline report');
    expect(report.json.total_records).toBeGreaterThanOrEqual(1);
    expect(report.json.by_category.length).toBeGreaterThanOrEqual(1);
    expect(report.json.data.map((row: any) => row.id)).toContain(recordID);

    expectXlsx(
      await xlsx(
        `/api/v1/exports/discipline?from=2026-09-01&to=2026-09-30&class_id=${seed.classAID}`,
        seed.adminToken,
      ),
      'discipline export',
    );

    const forbidden = await request('/api/v1/discipline/records', {
      method: 'POST',
      token: seed.teacherToken,
      body: {
        student_id: seed.student4ID,
        category_id: categoryID,
        occurred_at: `${ATTENDANCE_DATE}T10:00:00+07:00`,
        description: 'Di luar cakupan',
      },
    });
    expectStatus(forbidden, 403, 'teacher out-of-scope discipline');
  });

  test('dashboard and audit logs work', async () => {
    const dashboard = await request(
      `/api/v1/dashboard?date=${ATTENDANCE_DATE}&class_id=${seed.classAID}`,
      { token: seed.adminToken },
    );
    expectStatus(dashboard, 200, 'admin dashboard');
    expect(dashboard.json.date).toBe(ATTENDANCE_DATE);
    expect(dashboard.json.role).toBe('admin');
    expect(dashboard.json.attendance.total_students).toBe(3);
    expect(dashboard.json.attendance.recorded).toBe(3);
    expect(dashboard.json.attendance.present).toBe(0);
    expect(dashboard.json.attendance.late).toBe(2);
    expect(dashboard.json.attendance.absent).toBe(1);
    expect(dashboard.json.recent_activity.length).toBeGreaterThan(0);

    const principal = await request(`/api/v1/dashboard?date=${ATTENDANCE_DATE}`, {
      token: seed.principalToken,
    });
    expectStatus(principal, 200, 'principal dashboard');
    expect(principal.json.role).toBe('principal');

    const teacher = await request(`/api/v1/dashboard?date=${ATTENDANCE_DATE}`, {
      token: seed.teacherToken,
    });
    expectStatus(teacher, 200, 'teacher dashboard');
    expect(teacher.json.role).toBe('teacher');

    const audit = await request('/api/v1/audit-logs?action=user.create&limit=100', {
      token: seed.adminToken,
    });
    expectStatus(audit, 200, 'audit logs');
    expect(audit.json.total).toBeGreaterThanOrEqual(4);
    expect(audit.json.data.every((row: any) => row.action === 'user.create')).toBe(true);

    const dated = await request(
      `/api/v1/audit-logs?from=${ATTENDANCE_DATE}&to=${ATTENDANCE_DATE}&limit=100`,
      { token: seed.adminToken },
    );
    expectStatus(dated, 200, 'dated audit logs');
    expect(dated.json.limit).toBe(100);

    const forbidden = await request('/api/v1/audit-logs', { token: seed.teacherToken });
    expectStatus(forbidden, 403, 'teacher audit logs');
  });

  test('import templates, upload previews, and exports work', async () => {
    const imports = await request('/api/v1/imports', { token: seed.adminToken });
    expectStatus(imports, 200, 'list imports');
    expect(Array.isArray(imports.json.data)).toBe(true);

    const studentTemplate = await xlsx('/api/v1/import-templates/students', seed.adminToken);
    expectXlsx(studentTemplate, 'student import template');
    const teacherTemplate = await xlsx('/api/v1/import-templates/teachers', seed.adminToken);
    expectXlsx(teacherTemplate, 'teacher import template');

    const studentUpload = await uploadTemplate('/api/v1/imports/students', studentTemplate.bytes);
    expectStatus(studentUpload, 201, 'upload student template');
    expect(studentUpload.json.job.status).toBe('invalid');
    expect(studentUpload.json.job.id).toBeTruthy();
    const importID = studentUpload.json.job.id as string;

    const teacherUpload = await uploadTemplate('/api/v1/imports/teachers', teacherTemplate.bytes);
    expectStatus(teacherUpload, 201, 'upload teacher template');
    expect(teacherUpload.json.job.status).toBe('invalid');

    const detail = await request(`/api/v1/imports/${importID}`, { token: seed.adminToken });
    expectStatus(detail, 200, 'get import job');
    expect(detail.json.job.id).toBe(importID);

    const filtered = await request('/api/v1/imports?entity_type=students&status=invalid', {
      token: seed.adminToken,
    });
    expectStatus(filtered, 200, 'filter imports');
    expect(filtered.json.data.map((row: any) => row.id)).toContain(importID);

    const confirm = await request(`/api/v1/imports/${importID}/confirm`, {
      method: 'POST',
      token: seed.adminToken,
    });
    expectStatus(confirm, 409, 'confirm invalid import');

    const badFile = await fetch(`${API_URL}/api/v1/imports/students`, {
      method: 'POST',
      headers: { authorization: `Bearer ${seed.adminToken}` },
      body: (() => {
        const form = new FormData();
        form.set('file', new Blob(['not an xlsx'], { type: 'text/plain' }), 'students.txt');
        return form;
      })(),
    });
    expect(badFile.status).toBe(400);

    expectXlsx(await xlsx('/api/v1/exports/students', seed.adminToken), 'student export');
    expectXlsx(await xlsx('/api/v1/exports/teachers', seed.adminToken), 'teacher export');

    const forbidden = await request('/api/v1/imports', { token: seed.teacherToken });
    expectStatus(forbidden, 403, 'teacher list imports');
  });

  test('teacher presence: geofence, check-in, check-out, staff credential reset', async () => {
    const geofence = await request('/api/v1/geofence', { token: seed.principalToken });
    expectStatus(geofence, 200, 'read geofence');
    expect(geofence.json.latitude).toBe(SCHOOL_LAT);
    expect(geofence.json.longitude).toBe(SCHOOL_LNG);

    const forbiddenGeofence = await request('/api/v1/geofence', {
      method: 'PUT',
      token: seed.teacherToken,
      body: { latitude: SCHOOL_LAT, longitude: SCHOOL_LNG, radius_meters: 500, is_enabled: true },
    });
    expectStatus(forbiddenGeofence, 403, 'teacher update geofence');

    const badRadius = await request('/api/v1/geofence', {
      method: 'PUT',
      token: seed.adminToken,
      body: { latitude: SCHOOL_LAT, longitude: SCHOOL_LNG, radius_meters: 0, is_enabled: true },
    });
    expectStatus(badRadius, 400, 'geofence radius out of bounds');

    const updatedGeofence = await request('/api/v1/geofence', {
      method: 'PUT',
      token: seed.adminToken,
      body: { latitude: SCHOOL_LAT, longitude: SCHOOL_LNG, radius_meters: 500, is_enabled: true },
    });
    expectStatus(updatedGeofence, 200, 'update geofence');
    expect(updatedGeofence.json.radius_meters).toBe(500);

    const presenceUser = await createActivatedUser('e2e.presence', 'E2E Presence Teacher', ['teacher']);
    const presenceTeacherID = await createTeacher('E2E-T9', 'E2E Presence Teacher', presenceUser.id);

    const earlyCheckOut = await request('/api/v1/teacher-presence/check-out', {
      method: 'POST',
      token: presenceUser.token,
      body: { latitude: SCHOOL_LAT, longitude: SCHOOL_LNG },
    });
    expectStatus(earlyCheckOut, 409, 'check-out without check-in');

    const futureFix = await request('/api/v1/teacher-presence/check-in', {
      method: 'POST',
      token: presenceUser.token,
      body: { latitude: SCHOOL_LAT, longitude: SCHOOL_LNG, check_time: '2099-01-01T00:00:00Z' },
    });
    expectStatus(futureFix, 400, 'check-in with future check_time');

    const staleFix = await request('/api/v1/teacher-presence/check-in', {
      method: 'POST',
      token: presenceUser.token,
      body: {
        latitude: SCHOOL_LAT,
        longitude: SCHOOL_LNG,
        check_time: new Date(Date.now() - 48 * 60 * 60 * 1000).toISOString(),
      },
    });
    expectStatus(staleFix, 400, 'check-in older than 24h');

    const badCoordinates = await request('/api/v1/teacher-presence/check-in', {
      method: 'POST',
      token: presenceUser.token,
      body: { latitude: 95, longitude: SCHOOL_LNG },
    });
    expectStatus(badCoordinates, 400, 'check-in with invalid latitude');

    const forbiddenCheckIn = await request('/api/v1/teacher-presence/check-in', {
      method: 'POST',
      token: seed.principalToken,
      body: { latitude: SCHOOL_LAT, longitude: SCHOOL_LNG },
    });
    expectStatus(forbiddenCheckIn, 403, 'principal check-in');

    const noProfileCheckIn = await request('/api/v1/teacher-presence/check-in', {
      method: 'POST',
      token: seed.adminToken,
      body: { latitude: SCHOOL_LAT, longitude: SCHOOL_LNG },
    });
    expectStatus(noProfileCheckIn, 403, 'admin without teacher profile check-in');

    const checkIn = await request('/api/v1/teacher-presence/check-in', {
      method: 'POST',
      token: presenceUser.token,
      body: { latitude: SCHOOL_LAT, longitude: SCHOOL_LNG, accuracy: 10, notes: 'presence e2e' },
    });
    expectStatus(checkIn, 201, 'teacher check-in');
    expect(checkIn.json.teacher_id).toBe(presenceTeacherID);
    expect(checkIn.json.within_geofence).toBe(true);
    expect(checkIn.json.distance_meters).toBeLessThan(1);

    const doubleCheckIn = await request('/api/v1/teacher-presence/check-in', {
      method: 'POST',
      token: presenceUser.token,
      body: { latitude: SCHOOL_LAT, longitude: SCHOOL_LNG },
    });
    expectStatus(doubleCheckIn, 409, 'duplicate check-in same day');

    const earlyCheckOutAfter = await request('/api/v1/teacher-presence/check-out', {
      method: 'POST',
      token: presenceUser.token,
      body: {
        latitude: SCHOOL_LAT,
        longitude: SCHOOL_LNG,
        check_time: new Date(Date.now() - 3 * 60 * 60 * 1000).toISOString(),
      },
    });
    expectStatus(earlyCheckOutAfter, 400, 'check-out before check-in');

    const checkOut = await request('/api/v1/teacher-presence/check-out', {
      method: 'POST',
      token: presenceUser.token,
      body: { latitude: SCHOOL_LAT, longitude: SCHOOL_LNG },
    });
    expectStatus(checkOut, 201, 'teacher check-out');
    expect(checkOut.json.check_out_within_geofence).toBe(true);
    expect(checkOut.json.duration_minutes).toBeLessThan(5);

    const doubleCheckOut = await request('/api/v1/teacher-presence/check-out', {
      method: 'POST',
      token: presenceUser.token,
      body: { latitude: SCHOOL_LAT, longitude: SCHOOL_LNG },
    });
    expectStatus(doubleCheckOut, 409, 'duplicate check-out same day');

    const outsideCheckIn = await request('/api/v1/teacher-presence/check-in', {
      method: 'POST',
      token: seed.dutyToken,
      body: { latitude: -6.3, longitude: 106.816666 },
    });
    expectStatus(outsideCheckIn, 201, 'duty teacher outside-geofence check-in');
    expect(outsideCheckIn.json.within_geofence).toBe(false);
    expect(outsideCheckIn.json.distance_meters).toBeGreaterThan(500);

    const today = await request('/api/v1/teacher-presence/today', { token: presenceUser.token });
    expectStatus(today, 200, 'presence today');
    expect(today.json.teacher_id).toBe(presenceTeacherID);
    expect(today.json.check_in_at).toBeTruthy();
    expect(today.json.check_out_at).toBeTruthy();

    const history = await request(
      '/api/v1/teacher-presence/history?from=2020-01-01&to=2099-12-31&limit=100',
      { token: presenceUser.token },
    );
    expectStatus(history, 200, 'presence history');
    expect(history.json.total).toBe(1);
    expect(history.json.data[0].presence_date).toBe(checkIn.json.presence_date);

    const forbiddenHistory = await request('/api/v1/teacher-presence/history', {
      token: seed.principalToken,
    });
    expectStatus(forbiddenHistory, 403, 'principal presence history');

    const reset = await request(`/api/v1/users/${presenceUser.id}/credential-reset`, {
      method: 'POST',
      token: seed.adminToken,
    });
    expectStatus(reset, 200, 'staff credential reset');
    expect(reset.json.credential.purpose).toBe('reset');
    expect(reset.json.credential.token).toBeTruthy();

    const shortResetPassword = await request('/api/v1/auth/activate', {
      method: 'POST',
      body: { token: reset.json.credential.token, password: 'ElevenChars' },
    });
    expectStatus(shortResetPassword, 400, 'activate rejects password shorter than 12');
    expect(shortResetPassword.json.code).toBe('invalid_request');

    const activateReset = await request('/api/v1/auth/activate', {
      method: 'POST',
      body: { token: reset.json.credential.token, password: 'E2eResetPass123!' },
    });
    expectStatus(activateReset, 204, 'activate with reset token');

    const reusedResetToken = await request('/api/v1/auth/activate', {
      method: 'POST',
      body: { token: reset.json.credential.token, password: 'E2eAnotherPass123!' },
    });
    expectStatus(reusedResetToken, 400, 'reset token is single use');
    expect(reusedResetToken.json.code).toBe('invalid_token');

    const oldPasswordLogin = await request('/api/v1/auth/login', {
      method: 'POST',
      body: { login: 'e2e.presence', password: PASSWORD },
    });
    expectStatus(oldPasswordLogin, 401, 'login with the pre-reset password');

    const newPasswordLogin = await login('e2e.presence', 'E2eResetPass123!');
    expect(newPasswordLogin.token).toBeTruthy();
  });

  test('dispensations: create with document, verify, scoped reads', async () => {
    const jsonCreate = await createDispensation(seed.adminToken, {
      student_id: seed.student1ID,
      type: 'sick',
      reason: 'Demam e2e',
      start_date: ATTENDANCE_DATE,
      end_date: '2026-09-04',
    });
    expectStatus(jsonCreate, 201, 'create dispensation (json)');
    expect(jsonCreate.json.dispensation.status).toBe('pending');
    expect(jsonCreate.json.dispensation.class_id).toBe(seed.classAID);
    expect(jsonCreate.json.dispensation.document_available).toBe(false);
    const dispensation1ID = jsonCreate.json.dispensation.id as string;

    const documentCreate = await createDispensation(
      seed.teacherToken,
      {
        student_id: seed.student2ID,
        type: 'permit',
        reason: 'Klinik e2e',
        start_date: ATTENDANCE_DATE,
      },
      { filename: 'surat.pdf', contentType: 'application/pdf', bytes: pdfDocumentBytes() },
    );
    expectStatus(documentCreate, 201, 'create dispensation (multipart + pdf)');
    expect(documentCreate.json.dispensation.document_available).toBe(true);
    const documentDispensationID = documentCreate.json.dispensation.id as string;

    const otherClassCreate = await createDispensation(seed.teacherToken, {
      student_id: seed.student4ID,
      type: 'sick',
      reason: 'Di luar wali kelas',
      start_date: ATTENDANCE_DATE,
    });
    expectStatus(otherClassCreate, 403, 'teacher creates for other-class student');

    const principalCreate = await createDispensation(seed.principalToken, {
      student_id: seed.student1ID,
      type: 'sick',
      reason: 'Principal tidak berhak',
      start_date: ATTENDANCE_DATE,
    });
    expectStatus(principalCreate, 403, 'principal cannot create dispensations');

    const badType = await createDispensation(seed.adminToken, {
      student_id: seed.student1ID,
      type: 'vacation',
      reason: 'Tipe tidak dikenal',
      start_date: ATTENDANCE_DATE,
    });
    expectStatus(badType, 400, 'invalid dispensation type');

    const reversedDates = await createDispensation(seed.adminToken, {
      student_id: seed.student1ID,
      type: 'sick',
      reason: 'Rentang terbalik',
      start_date: '2026-09-05',
      end_date: ATTENDANCE_DATE,
    });
    expectStatus(reversedDates, 400, 'end before start');

    const badDocument = await createDispensation(
      seed.teacherToken,
      {
        student_id: seed.student2ID,
        type: 'sick',
        reason: 'Dokumen salah format',
        start_date: ATTENDANCE_DATE,
      },
      { filename: 'catatan.txt', contentType: 'text/plain', bytes: new TextEncoder().encode('bukan pdf') },
    );
    expectStatus(badDocument, 400, 'document extension rejected');

    const classBOther = await createDispensation(seed.adminToken, {
      student_id: seed.student4ID,
      type: 'late',
      reason: 'Keperluan keluarga',
      start_date: ATTENDANCE_DATE,
    });
    expectStatus(classBOther, 201, 'admin creates class B dispensation');
    const classBDispensationID = classBOther.json.dispensation.id as string;

    const list = await request(
      `/api/v1/dispensations?student_id=${seed.student1ID}&status=pending&limit=100`,
      { token: seed.adminToken },
    );
    expectStatus(list, 200, 'list dispensations');
    expect(list.json.data.map((row: any) => row.id)).toContain(dispensation1ID);

    const invalidStatus = await request('/api/v1/dispensations?status=bogus', { token: seed.adminToken });
    expectStatus(invalidStatus, 400, 'invalid status filter rejected');

    const scopedDetail = await request(`/api/v1/dispensations/${dispensation1ID}`, {
      token: seed.teacherToken,
    });
    expectStatus(scopedDetail, 200, 'homeroom teacher reads own-class dispensation');

    const forbiddenDetail = await request(`/api/v1/dispensations/${classBDispensationID}`, {
      token: seed.teacherToken,
    });
    expectStatus(forbiddenDetail, 403, 'teacher cannot read other-class dispensation');

    const forbiddenVerify = await request(`/api/v1/dispensations/${classBDispensationID}/verify`, {
      method: 'POST',
      token: seed.teacherToken,
      body: { status: 'approved', note: 'bukan kelas saya' },
    });
    expectStatus(forbiddenVerify, 403, 'teacher verifies other-class dispensation');

    const dutyVerify = await request(`/api/v1/dispensations/${classBDispensationID}/verify`, {
      method: 'POST',
      token: seed.dutyToken,
      body: { status: 'approved', note: 'disetujui guru piket' },
    });
    expectStatus(dutyVerify, 200, 'duty teacher verifies school-wide');
    expect(dutyVerify.json.dispensation.status).toBe('approved');
    expect(dutyVerify.json.dispensation.verified_by).toBeTruthy();

    const verifyTwice = await request(`/api/v1/dispensations/${classBDispensationID}/verify`, {
      method: 'POST',
      token: seed.adminToken,
      body: { status: 'rejected' },
    });
    expectStatus(verifyTwice, 409, 'verify after decision');

    const badDecision = await request(`/api/v1/dispensations/${dispensation1ID}/verify`, {
      method: 'POST',
      token: seed.adminToken,
      body: { status: 'pending' },
    });
    expectStatus(badDecision, 400, 'pending is not a decision');

    const verify = await request(`/api/v1/dispensations/${dispensation1ID}/verify`, {
      method: 'POST',
      token: seed.adminToken,
      body: { status: 'approved', note: 'dipercaya e2e' },
    });
    expectStatus(verify, 200, 'verify dispensation');
    expect(verify.json.dispensation.note).toBe('dipercaya e2e');

    const document = await downloadBinary(`/api/v1/dispensations/${documentDispensationID}/document`, seed.adminToken);
    expect(document.res.status).toBe(200);
    expect(document.res.headers.get('content-type')).toContain('application/pdf');
    expect(document.res.headers.get('x-content-type-options')).toBe('nosniff');
    expect(String.fromCharCode(...document.bytes.slice(0, 4))).toBe('%PDF');

    const missingDocument = await request(`/api/v1/dispensations/${dispensation1ID}/document`, {
      token: seed.adminToken,
    });
    expectStatus(missingDocument, 404, 'document download without a document');

    const studentPortalForbidden = await request('/api/v1/my/dispensations', { token: seed.teacherToken });
    expectStatus(studentPortalForbidden, 403, 'teacher cannot use the student portal');
  });

  test('duty rotations and uniform reports', async () => {
    const rotation = await request('/api/v1/duty-rotations', {
      method: 'POST',
      token: seed.adminToken,
      body: {
        duty_assignment_id: seed.dutyID,
        substitute_teacher_id: seed.teacherBID,
        date: ROTATION_WED_1,
        reason: 'Guru piket sakit',
      },
    });
    expectStatus(rotation, 201, 'create duty rotation');
    expect(rotation.json.duty_rotation.status).toBe('substituted');
    expect(rotation.json.duty_rotation.weekday).toBe(3);
    expect(rotation.json.duty_rotation.original_teacher_id).toBe(seed.teacher2ID);
    expect(rotation.json.duty_rotation.substitute_teacher_name).toBe('E2E Substitute Teacher Updated');
    const rotationID = rotation.json.duty_rotation.id as string;

    const duplicate = await request('/api/v1/duty-rotations', {
      method: 'POST',
      token: seed.adminToken,
      body: {
        duty_assignment_id: seed.dutyID,
        substitute_teacher_id: seed.teacherBID,
        date: ROTATION_WED_1,
        reason: 'Duplikat',
      },
    });
    expectStatus(duplicate, 409, 'duplicate rotation rejected');

    const wrongWeekday = await request('/api/v1/duty-rotations', {
      method: 'POST',
      token: seed.adminToken,
      body: {
        duty_assignment_id: seed.dutyID,
        substitute_teacher_id: seed.teacherBID,
        date: '2026-09-01',
        reason: 'Bukan hari piket',
      },
    });
    expectStatus(wrongWeekday, 400, 'rotation date must match assignment weekday');

    const missingAssignment = await request('/api/v1/duty-rotations', {
      method: 'POST',
      token: seed.adminToken,
      body: {
        duty_assignment_id: UNKNOWN_TEACHER_UUID,
        substitute_teacher_id: seed.teacherBID,
        date: ROTATION_WED_1,
        reason: 'Assignment tidak ada',
      },
    });
    expectStatus(missingAssignment, 404, 'unknown duty assignment');

    const selfSubstitute = await request('/api/v1/duty-rotations', {
      method: 'POST',
      token: seed.adminToken,
      body: {
        duty_assignment_id: seed.dutyID,
        substitute_teacher_id: seed.teacher2ID,
        date: ROTATION_WED_1,
        reason: 'Menggantikan diri sendiri',
      },
    });
    expectStatus(selfSubstitute, 400, 'substitute must differ from original');

    const irrelevantCreate = await request('/api/v1/duty-rotations', {
      method: 'POST',
      token: seed.teacherToken,
      body: {
        duty_assignment_id: seed.dutyID,
        substitute_teacher_id: seed.teacherBID,
        date: ROTATION_WED_2,
        reason: 'Bukan piket saya',
      },
    });
    expectStatus(irrelevantCreate, 403, 'unrelated teacher cannot substitute');

    const ownRotation = await request('/api/v1/duty-rotations', {
      method: 'POST',
      token: seed.dutyToken,
      body: {
        duty_assignment_id: seed.dutyID,
        substitute_teacher_id: seed.teacherBID,
        date: ROTATION_WED_2,
        reason: 'Silih sendiri',
      },
    });
    expectStatus(ownRotation, 201, 'duty teacher substitutes own assignment');
    const ownRotationID = ownRotation.json.duty_rotation.id as string;

    const irrelevantCancel = await request(`/api/v1/duty-rotations/${ownRotationID}/cancel`, {
      method: 'PATCH',
      token: seed.teacherToken,
      body: { reason: 'Tidak relevan' },
    });
    expectStatus(irrelevantCancel, 403, 'unrelated teacher cannot cancel');

    const cancel = await request(`/api/v1/duty-rotations/${ownRotationID}/cancel`, {
      method: 'PATCH',
      token: seed.dutyToken,
      body: { reason: 'Jadi hadir' },
    });
    expectStatus(cancel, 200, 'cancel rotation');
    expect(cancel.json.duty_rotation.status).toBe('cancelled');
    expect(cancel.json.duty_rotation.reason).toBe('Jadi hadir');

    const cancelTwice = await request(`/api/v1/duty-rotations/${ownRotationID}/cancel`, {
      method: 'PATCH',
      token: seed.adminToken,
      body: {},
    });
    expectStatus(cancelTwice, 409, 'cancel twice');

    const adminList = await request(`/api/v1/duty-rotations?date=${ROTATION_WED_1}`, {
      token: seed.adminToken,
    });
    expectStatus(adminList, 200, 'admin lists rotations');
    expect(adminList.json.data.map((row: any) => row.id)).toContain(rotationID);

    const dutyList = await request(`/api/v1/duty-rotations?date=${ROTATION_WED_1}`, {
      token: seed.dutyToken,
    });
    expectStatus(dutyList, 200, 'duty teacher lists own rotations');
    expect(dutyList.json.data.map((row: any) => row.id)).toContain(rotationID);

    const teacherList = await request(`/api/v1/duty-rotations?date=${ROTATION_WED_1}`, {
      token: seed.teacherToken,
    });
    expectStatus(teacherList, 200, 'unrelated teacher list is scoped');
    expect(teacherList.json.data).toHaveLength(0);

    const uniform = await request('/api/v1/uniform-reports', {
      method: 'POST',
      token: seed.dutyToken,
      body: {
        for_date: ATTENDANCE_DATE,
        teacher_id: seed.teacher2ID,
        scope: 'student',
        target_student_id: seed.student1ID,
        note: 'Dasi tertinggal',
      },
    });
    expectStatus(uniform, 201, 'duty teacher records uniform report');
    expect(uniform.json.uniform_report.scope).toBe('student');
    expect(uniform.json.uniform_report.target_student_id).toBe(seed.student1ID);

    const uniformForbidden = await request('/api/v1/uniform-reports', {
      method: 'POST',
      token: seed.teacherToken,
      body: { for_date: ATTENDANCE_DATE, teacher_id: seed.teacher1ID, scope: 'teacher' },
    });
    expectStatus(uniformForbidden, 403, 'plain teacher cannot write uniform reports');

    const uniformReadForbidden = await request('/api/v1/uniform-reports', { token: seed.principalToken });
    expectStatus(uniformReadForbidden, 403, 'principal cannot read uniform reports');

    const uniformBadScope = await request('/api/v1/uniform-reports', {
      method: 'POST',
      token: seed.dutyToken,
      body: {
        for_date: ATTENDANCE_DATE,
        teacher_id: seed.teacher2ID,
        scope: 'teacher',
        target_student_id: seed.student1ID,
      },
    });
    expectStatus(uniformBadScope, 400, 'teacher-scope note must not target a student');

    const uniformUnknownTeacher = await request('/api/v1/uniform-reports', {
      method: 'POST',
      token: seed.dutyToken,
      body: { for_date: ATTENDANCE_DATE, teacher_id: UNKNOWN_TEACHER_UUID, scope: 'teacher' },
    });
    expectStatus(uniformUnknownTeacher, 404, 'unknown teacher for uniform report');

    const uniformList = await request(
      `/api/v1/uniform-reports?date=${ATTENDANCE_DATE}&teacher_id=${seed.teacher2ID}`,
      { token: seed.adminToken },
    );
    expectStatus(uniformList, 200, 'list uniform reports');
    expect(uniformList.json.data).toHaveLength(1);
    expect(uniformList.json.data[0].note).toBe('Dasi tertinggal');
  });

  test('sanction rules: CRUD, replace-all and overlap conflicts', async () => {
    const emptyList = await request('/api/v1/sanction-rules', { token: seed.teacherToken });
    expectStatus(emptyList, 200, 'staff can read sanction rules');
    expect(Array.isArray(emptyList.json.data)).toBe(true);

    const forbiddenWrite = await request('/api/v1/sanction-rules', {
      method: 'POST',
      token: seed.dutyToken,
      body: { name: 'Tanpa hak', min_points: 0, max_points: 5, action: 'Teguran' },
    });
    expectStatus(forbiddenWrite, 403, 'duty teacher cannot write sanction rules');

    const create = await request('/api/v1/sanction-rules', {
      method: 'POST',
      token: seed.adminToken,
      body: { name: 'Teguran lisan', min_points: 0, max_points: 10, action: 'Teguran lisan', level: '1' },
    });
    expectStatus(create, 201, 'create sanction rule');
    expect(create.json.sanction_rule.is_active).toBe(true);
    const ruleID = create.json.sanction_rule.id as string;

    const overlap = await request('/api/v1/sanction-rules', {
      method: 'POST',
      token: seed.adminToken,
      body: { name: 'Tumpang tindih', min_points: 5, max_points: 15, action: 'Teguran tertulis' },
    });
    expectStatus(overlap, 409, 'overlapping range rejected');

    const invertedRange = await request('/api/v1/sanction-rules', {
      method: 'POST',
      token: seed.adminToken,
      body: { name: 'Rentang terbalik', min_points: 10, max_points: 3, action: 'Invalid' },
    });
    expectStatus(invertedRange, 400, 'max_points must exceed min_points');

    const openTop = await request('/api/v1/sanction-rules', {
      method: 'POST',
      token: seed.adminToken,
      body: { name: 'Pengeluaran', min_points: 11, action: 'Surat peringatan', level: '2' },
    });
    expectStatus(openTop, 201, 'open-ended top tier');
    const openTopID = openTop.json.sanction_rule.id as string;

    const collideUpdate = await request(`/api/v1/sanction-rules/${ruleID}`, {
      method: 'PUT',
      token: seed.adminToken,
      body: { name: 'Teguran lisan', min_points: 12, action: 'Teguran lisan' },
    });
    expectStatus(collideUpdate, 409, 'update into another active range');

    const update = await request(`/api/v1/sanction-rules/${ruleID}`, {
      method: 'PUT',
      token: seed.adminToken,
      body: { name: 'Teguran lisan diperbarui', min_points: 0, max_points: 10, action: 'Teguran lisan', level: '1' },
    });
    expectStatus(update, 200, 'update sanction rule');
    expect(update.json.sanction_rule.name).toBe('Teguran lisan diperbarui');

    const detail = await request(`/api/v1/sanction-rules/${ruleID}`, { token: seed.principalToken });
    expectStatus(detail, 200, 'get sanction rule');
    expect(detail.json.sanction_rule.id).toBe(ruleID);

    const deactivateTop = await request(`/api/v1/sanction-rules/${openTopID}/status`, {
      method: 'PATCH',
      token: seed.adminToken,
      body: { is_active: false },
    });
    expectStatus(deactivateTop, 200, 'deactivate rule');
    expect(deactivateTop.json.sanction_rule.is_active).toBe(false);

    const reuseInactiveRange = await request('/api/v1/sanction-rules', {
      method: 'POST',
      token: seed.adminToken,
      body: { name: 'Rentang baru', min_points: 11, max_points: 20, action: 'Panggilan orang tua' },
    });
    expectStatus(reuseInactiveRange, 201, 'inactive ranges do not collide');
    const reuseID = reuseInactiveRange.json.sanction_rule.id as string;

    const reactivateCollision = await request(`/api/v1/sanction-rules/${openTopID}/status`, {
      method: 'PATCH',
      token: seed.adminToken,
      body: { is_active: true },
    });
    expectStatus(reactivateCollision, 409, 'reactivation re-checks overlaps');

    const deactivateReuse = await request(`/api/v1/sanction-rules/${reuseID}/status`, {
      method: 'PATCH',
      token: seed.adminToken,
      body: { is_active: false },
    });
    expectStatus(deactivateReuse, 200, 'deactivate reused range');

    const reactivate = await request(`/api/v1/sanction-rules/${openTopID}/status`, {
      method: 'PATCH',
      token: seed.adminToken,
      body: { is_active: true },
    });
    expectStatus(reactivate, 200, 'reactivate after conflict cleared');

    const deleteRemoved = await request(`/api/v1/sanction-rules/${reuseID}`, {
      method: 'DELETE',
      token: seed.adminToken,
    });
    expectStatus(deleteRemoved, 204, 'delete sanction rule');
    const missing = await request(`/api/v1/sanction-rules/${reuseID}`, { token: seed.adminToken });
    expectStatus(missing, 404, 'deleted rule is gone');

    const replaceOverlap = await request('/api/v1/sanction-rules', {
      method: 'PUT',
      token: seed.adminToken,
      body: {
        rules: [
          { min_points: 0, max_points: 10, action: 'Pembinaan' },
          { min_points: 5, max_points: 15, action: 'Surat peringatan' },
        ],
      },
    });
    expectStatus(replaceOverlap, 409, 'replace payload with internal overlap rejected');

    const missingRulesKey = await request('/api/v1/sanction-rules', {
      method: 'PUT',
      token: seed.adminToken,
      body: {},
    });
    expectStatus(missingRulesKey, 400, 'replace requires a rules array');

    const replace = await request('/api/v1/sanction-rules', {
      method: 'PUT',
      token: seed.adminToken,
      body: {
        rules: [
          { min_points: 0, max_points: 10, action: 'Pembinaan', level: '1' },
          { min_points: 11, action: 'Surat peringatan', level: '2' },
        ],
      },
    });
    expectStatus(replace, 200, 'replace all rules');
    expect(replace.json.data).toHaveLength(2);

    const studentSummary = await request(`/api/v1/discipline/students/${seed.student1ID}`, {
      token: seed.adminToken,
    });
    expectStatus(studentSummary, 200, 'student summary with current level');
    expect(studentSummary.json.total_points).toBeGreaterThanOrEqual(8);
    expect(studentSummary.json.current_level).not.toBeNull();
    expect(studentSummary.json.current_level.action).toBe('Pembinaan');
  });

  test('employment types and teacher employment assignment', async () => {
    const forbidden = await request('/api/v1/employment-types', { token: seed.teacherToken });
    expectStatus(forbidden, 403, 'teacher cannot read employment types');

    const list = await request('/api/v1/employment-types', { token: seed.adminToken });
    expectStatus(list, 200, 'list employment types');
    const codes = list.json.data.map((row: any) => row.code);
    expect(codes).toEqual(expect.arrayContaining(['permanent', 'honorary', 'staff_tu']));
    const permanent = list.json.data.find((row: any) => row.code === 'permanent');

    const create = await request('/api/v1/employment-types', {
      method: 'POST',
      token: seed.adminToken,
      body: { code: 'e2e_contract', name: 'Kontrak E2E', description: 'Jenis kepegawaian uji e2e' },
    });
    expectStatus(create, 201, 'create employment type');
    expect(create.json.employment_type.is_active).toBe(true);
    const employmentID = create.json.employment_type.id as string;

    const duplicate = await request('/api/v1/employment-types', {
      method: 'POST',
      token: seed.adminToken,
      body: { code: 'e2e_contract', name: 'Duplikat' },
    });
    expectStatus(duplicate, 409, 'duplicate code rejected');

    const detail = await request(`/api/v1/employment-types/${employmentID}`, { token: seed.adminToken });
    expectStatus(detail, 200, 'get employment type');

    const update = await request(`/api/v1/employment-types/${employmentID}`, {
      method: 'PUT',
      token: seed.adminToken,
      body: { code: 'e2e_contract', name: 'Kontrak E2E Diperbarui' },
    });
    expectStatus(update, 200, 'update employment type');
    expect(update.json.employment_type.name).toBe('Kontrak E2E Diperbarui');

    const deactivate = await request(`/api/v1/employment-types/${employmentID}/status`, {
      method: 'PATCH',
      token: seed.adminToken,
      body: { is_active: false },
    });
    expectStatus(deactivate, 200, 'deactivate employment type');
    expect(deactivate.json.employment_type.is_active).toBe(false);

    const teacherUpdate = await request(`/api/v1/teachers/${seed.teacher1ID}`, {
      method: 'PUT',
      token: seed.adminToken,
      body: {
        employee_number: 'E2E-T1',
        full_name: 'E2E Teacher',
        email: 'e2e-t1@example.test',
        employment_type_id: permanent.id,
      },
    });
    expectStatus(teacherUpdate, 200, 'assign employment type to teacher');
    expect(teacherUpdate.json.teacher.employment_type_id).toBe(permanent.id);
    expect(teacherUpdate.json.teacher.employment_type_name).toBe('Permanen (PNS/PPPK)');

    const dashboard = await request(`/api/v1/dashboard?date=${ATTENDANCE_DATE}`, { token: seed.adminToken });
    expectStatus(dashboard, 200, 'dashboard employment breakdown');
    expect(dashboard.json.active_teachers_breakdown.permanent).toBeGreaterThanOrEqual(1);
  });

  test('student accounts: provision, activate, /my portal, deactivate', async () => {
    const noAccount = await request(`/api/v1/students/${seed.student1ID}/account`, { token: seed.adminToken });
    expectStatus(noAccount, 200, 'account lookup before provisioning');
    expect(noAccount.json.account.user_id).toBeNull();

    const forbiddenProvision = await request(`/api/v1/students/${seed.student1ID}/account`, {
      method: 'POST',
      token: seed.dutyToken,
      body: { username: 'e2e.notallowed' },
    });
    expectStatus(forbiddenProvision, 403, 'only admin provisions student accounts');

    const provision = await request(`/api/v1/students/${seed.student1ID}/account`, {
      method: 'POST',
      token: seed.adminToken,
      body: { username: 'e2e.student1', email: 'e2e.student1@example.test' },
    });
    expectStatus(provision, 201, 'provision student account');
    expect(provision.json.user.status).toBe('pending');
    expect(provision.json.user.roles).toEqual(['student']);
    expect(provision.json.credential.purpose).toBe('invite');
    expect(provision.json.credential.token).toBeTruthy();

    const secondProvision = await request(`/api/v1/students/${seed.student1ID}/account`, {
      method: 'POST',
      token: seed.adminToken,
      body: {},
    });
    expectStatus(secondProvision, 409, 'provisioning a linked student conflicts');

    const shortPassword = await request('/api/v1/auth/activate', {
      method: 'POST',
      body: { token: provision.json.credential.token, password: 'ElevenChars' },
    });
    expectStatus(shortPassword, 400, 'invite activation enforces 12-character minimum');

    const activate = await request('/api/v1/auth/activate', {
      method: 'POST',
      body: { token: provision.json.credential.token, password: 'E2eStudentPass123!' },
    });
    expectStatus(activate, 204, 'activate student invite');

    const reusedToken = await request('/api/v1/auth/activate', {
      method: 'POST',
      body: { token: provision.json.credential.token, password: 'E2eStudentPass456!' },
    });
    expectStatus(reusedToken, 400, 'invite token is single use');
    expect(reusedToken.json.code).toBe('invalid_token');

    const studentSession = await login('e2e.student1', 'E2eStudentPass123!');
    const studentToken = studentSession.token;

    const linkedAccount = await request(`/api/v1/students/${seed.student1ID}/account`, { token: seed.adminToken });
    expectStatus(linkedAccount, 200, 'account lookup after activation');
    expect(linkedAccount.json.account.username).toBe('e2e.student1');
    expect(linkedAccount.json.account.status).toBe('active');

    const profile = await request('/api/v1/my/profile', { token: studentToken });
    expectStatus(profile, 200, 'student profile');
    expect(profile.json.profile.student_id).toBe(seed.student1ID);
    expect(profile.json.profile.class_id).toBe(seed.classAID);

    const myAttendance = await request(
      `/api/v1/my/attendance?from=2026-09-01&to=2026-09-30&limit=100`,
      { token: studentToken },
    );
    expectStatus(myAttendance, 200, 'my attendance');
    expect(myAttendance.json.total).toBeGreaterThanOrEqual(1);
    expect(myAttendance.json.data.some((row: any) => row.status === 'late')).toBe(true);

    const mySummaryAlias = await request(
      `/api/v1/my/attendance-history?from=2026-09-01&to=2026-09-30`,
      { token: studentToken },
    );
    expectStatus(mySummaryAlias, 200, 'attendance-history alias');
    expect(mySummaryAlias.json.total).toBe(myAttendance.json.total);

    const myAttendanceSummary = await request('/api/v1/my/attendance-summary?from=2026-09-01&to=2026-09-30', {
      token: studentToken,
    });
    expectStatus(myAttendanceSummary, 200, 'my attendance summary');

    const myDiscipline = await request('/api/v1/my/discipline?from=2026-09-01&to=2026-09-30', {
      token: studentToken,
    });
    expectStatus(myDiscipline, 200, 'my discipline records');
    expect(myDiscipline.json.total).toBeGreaterThanOrEqual(1);

    const myDisciplineSummary = await request('/api/v1/my/discipline-summary?from=2026-09-01&to=2026-09-30', {
      token: studentToken,
    });
    expectStatus(myDisciplineSummary, 200, 'my discipline summary');
    expect(myDisciplineSummary.json.total_points).toBeGreaterThanOrEqual(8);

    const myDispensation = await request('/api/v1/my/dispensations', {
      method: 'POST',
      token: studentToken,
      body: {
        student_id: seed.student4ID,
        type: 'sick',
        reason: 'Permisi dari siswa',
        start_date: '2026-09-15',
      },
    });
    expectStatus(myDispensation, 201, 'student requests own dispensation');
    expect(myDispensation.json.dispensation.student_id).toBe(seed.student1ID);

    const myDispensationList = await request('/api/v1/my/dispensations?status=pending', { token: studentToken });
    expectStatus(myDispensationList, 200, 'student lists own dispensations');
    expect(myDispensationList.json.total).toBe(1);

    const teacherPortalForbidden = await request('/api/v1/my/profile', { token: seed.teacherToken });
    expectStatus(teacherPortalForbidden, 403, 'teachers have no /my portal');

    const lonely = await createActivatedUser('e2e.lonely', 'E2E Lonely Student', ['student']);
    const lonelyProfile = await request('/api/v1/my/profile', { token: lonely.token });
    expectStatus(lonelyProfile, 403, 'unlinked student login is rejected');

    const inactiveStudent = await request(`/api/v1/students/${seed.student5ID}/account`, {
      method: 'POST',
      token: seed.adminToken,
      body: {},
    });
    expectStatus(inactiveStudent, 409, 'inactive students cannot be provisioned');

    const deactivateAccount = await request(`/api/v1/students/${seed.student1ID}/account`, {
      method: 'DELETE',
      token: seed.adminToken,
    });
    expectStatus(deactivateAccount, 204, 'deactivate student account');

    const afterDeactivate = await request('/api/v1/my/profile', { token: studentToken });
    // DELETE /students/:id/account deactivates the login user and revokes its
    // sessions (docs/API_ADDENDUM.md), so the stale JWT fails authentication
    // outright — 401, not the 403 a linked-but-profile-less user gets.
    expectStatus(afterDeactivate, 401, 'portal blocked after account deactivation');

    const clearedAccount = await request(`/api/v1/students/${seed.student1ID}/account`, { token: seed.adminToken });
    expectStatus(clearedAccount, 200, 'account lookup after deactivation');
    expect(clearedAccount.json.account.user_id).toBeNull();

    const reprovision = await request(`/api/v1/students/${seed.student1ID}/account`, {
      method: 'POST',
      token: seed.adminToken,
      body: { username: 'e2e.student1.again' },
    });
    expectStatus(reprovision, 201, 're-provisioning after unlink works');
  });

  test('audit logs record wave-2 write actions', async () => {
    const actions = [
      'geofence.update',
      'dispensation.verify',
      'duty.substitution',
      'duty.substitution.cancel',
      'sanction_rule.create',
      'sanction_rule.replace',
      'employment_type.create',
      'student_account.create',
      'student_account.delete',
      'user.password.reset',
    ];
    for (const action of actions) {
      const logs = await request(`/api/v1/audit-logs?action=${action}&limit=100`, { token: seed.adminToken });
      expectStatus(logs, 200, `audit logs for ${action}`);
      expect(logs.json.total).toBeGreaterThanOrEqual(1);
      expect(logs.json.data.every((row: any) => row.action === action)).toBe(true);
    }

    const forbidden = await request('/api/v1/audit-logs', { token: seed.dutyToken });
    expectStatus(forbidden, 403, 'duty teacher cannot read audit logs');
  });

  test('metrics reflect completed API traffic', async () => {
    const metrics = await request('/metrics');
    expectStatus(metrics, 200, 'metrics after traffic');
    expect(metrics.text).toContain('ifter_http_requests_total');
  });
});
