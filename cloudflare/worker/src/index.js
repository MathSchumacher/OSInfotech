const PBKDF2_ITERATIONS = 100000;
const SESSION_DAYS = 30;
const roles = new Set(["admin", "technician"]);
const syncEntityTypes = new Set(["customer", "service_order", "sales_term", "checklist_template"]);
const maxSyncChanges = 200;

export default {
  async fetch(request, env) {
    try {
      return await route(request, env);
    } catch (error) {
      if (error instanceof APIError) {
        return json({ error: error.error, message: error.message }, error.status);
      }
      console.error("Unhandled API error", error);
      return json({ error: "internal_error", message: "Erro interno da API." }, 500);
    }
  },
};

async function route(request, env) {
  const url = new URL(request.url);
  const path = url.pathname.replace(/\/$/, "") || "/";

  if (request.method === "GET" && path === "/health") {
    return json({ ok: true, service: "osinfotech-api", now: new Date().toISOString() });
  }
  if (request.method === "POST" && path === "/v1/setup") {
    return setup(request, env);
  }
  if (request.method === "GET" && path === "/v1/setup/status") {
    return setupStatus(env);
  }
  if (request.method === "POST" && path === "/v1/auth/login") {
    return login(request, env);
  }
  if (request.method === "POST" && path === "/v1/auth/logout") {
    return logout(request, env);
  }
  if (request.method === "GET" && path === "/v1/me") {
    const identity = await requireIdentity(request, env);
    return json({ user: publicUser(identity) });
  }
  if (path === "/v1/organization") {
    const identity = await requireIdentity(request, env);
    if (request.method === "GET") {
      return organizationStatus(identity, env);
    }
    if (request.method === "PATCH") {
      const admin = await requireAdmin(request, env);
      return updateOrganization(request, admin, env);
    }
  }
  if (path === "/v1/sync/pull" && request.method === "GET") {
    const identity = await requireIdentity(request, env);
    return pullChanges(request, identity, env);
  }
  if (path === "/v1/sync/push" && request.method === "POST") {
    const identity = await requireIdentity(request, env);
    return pushChanges(request, identity, env);
  }
  if (path === "/v1/users") {
    const identity = await requireAdmin(request, env);
    if (request.method === "GET") {
      return listUsers(identity, env);
    }
    if (request.method === "POST") {
      return createUser(request, identity, env);
    }
  }
  const userMatch = path.match(/^\/v1\/users\/([^/]+)$/);
  if (userMatch && request.method === "PATCH") {
    const identity = await requireAdmin(request, env);
    return updateUser(request, identity, decodeURIComponent(userMatch[1]), env);
  }

  return json({ error: "not_found", message: "Rota nao encontrada." }, 404);
}

async function setupStatus(env) {
  const existing = await env.DB.prepare("SELECT COUNT(*) AS count FROM users").first();
  return json({ initialized: Number(existing?.count || 0) > 0 });
}

async function setup(request, env) {
  const setupToken = request.headers.get("X-Setup-Token") || "";
  if (!env.SETUP_TOKEN || !constantTimeEqual(setupToken, env.SETUP_TOKEN)) {
    return json({ error: "forbidden", message: "Token de configuracao invalido." }, 403);
  }

  const existing = await env.DB.prepare("SELECT COUNT(*) AS count FROM users").first();
  if (Number(existing?.count || 0) > 0) {
    return json({ error: "already_initialized", message: "A organizacao ja foi configurada." }, 409);
  }

  const body = await readJSON(request);
  const organizationName = text(body.organization_name, "Nome da empresa");
  const name = text(body.name);
  const loginName = normalizeLogin(body.login);
  const password = text(body.password);
  const validation = validateUserInput(name, loginName, password, "admin");
  if (validation) return json(validation, 400);

  const now = new Date().toISOString();
  const organizationID = crypto.randomUUID();
  const userID = crypto.randomUUID();
  const passwordHash = await hashPassword(password);
  await env.DB.prepare("INSERT INTO organizations(id, name, created_at, updated_at) VALUES (?, ?, ?, ?)").bind(
      organizationID,
      organizationName,
      now,
      now,
    ).run();
  try {
    await env.DB.prepare(`
      INSERT INTO users(id, organization_id, name, login, password_hash, role, active, created_at, updated_at)
      VALUES (?, ?, ?, ?, ?, 'admin', 1, ?, ?)
    `).bind(userID, organizationID, name, loginName, passwordHash, now, now).run();
  } catch (error) {
    await env.DB.prepare("DELETE FROM organizations WHERE id = ?").bind(organizationID).run();
    throw error;
  }

  const identity = { id: userID, organizationID, name, login: loginName, role: "admin", active: true };
  await writeAudit(env, identity, "organization.setup", "organization", organizationID, { name: organizationName });
  const session = await createSession(identity, env);
  return json({ user: publicUser(identity), ...session }, 201);
}

async function login(request, env) {
  const body = await readJSON(request);
  const loginName = normalizeLogin(body.login);
  const password = text(body.password);
  if (!loginName || !password) {
    return json({ error: "invalid_credentials", message: "Login ou senha invalidos." }, 401);
  }

  // A conferencia do bloqueio vem antes de qualquer PBKDF2. Cada tentativa
  // custa 100 mil iteracoes no worker, entao adivinhacao de senha era ataque a
  // conta e conta de CPU ao mesmo tempo.
  const blocked = await loginIsBlocked(env, loginName);
  if (blocked) {
    return json({
      error: "too_many_attempts",
      message: "Muitas tentativas. Tente de novo em alguns minutos.",
    }, 429);
  }

  const user = await env.DB.prepare(`
    SELECT id, organization_id, name, login, password_hash, role, active
    FROM users
    WHERE login = ? AND active = 1
    LIMIT 1
  `).bind(loginName).first();
  if (!user || !(await verifyPassword(password, user.password_hash))) {
    await registerFailedLogin(env, loginName);
    return json({ error: "invalid_credentials", message: "Login ou senha invalidos." }, 401);
  }

  await clearLoginAttempts(env, loginName);
  const identity = toIdentity(user);
  const session = await createSession(identity, env);
  return json({ user: publicUser(identity), ...session });
}

// Quantas tentativas erradas seguidas travam a conta, e por quanto tempo. Oito
// da folga para quem digita errado no balcao e ainda assim torna inviavel
// varrer senhas.
const maxLoginAttempts = 8;
const loginBlockMinutes = 15;

async function loginIsBlocked(env, loginName) {
  const row = await env.DB.prepare(
    "SELECT blocked_until FROM login_attempts WHERE login_name = ?",
  ).bind(loginName).first();
  if (!row || !row.blocked_until) return false;
  return new Date(row.blocked_until).getTime() > Date.now();
}

async function registerFailedLogin(env, loginName) {
  const now = new Date();
  const row = await env.DB.prepare(
    "SELECT attempts, first_attempt_at FROM login_attempts WHERE login_name = ?",
  ).bind(loginName).first();

  // Janela expirada conta como comeco de vez: quem errou uma vez semana
  // passada nao pode chegar mais perto do bloqueio por causa disso.
  const windowStart = row ? new Date(row.first_attempt_at).getTime() : 0;
  const fresh = !row || now.getTime() - windowStart > loginBlockMinutes * 60000;
  const attempts = fresh ? 1 : Number(row.attempts || 0) + 1;
  const firstAt = fresh ? now.toISOString() : row.first_attempt_at;
  const blockedUntil = attempts >= maxLoginAttempts
    ? new Date(now.getTime() + loginBlockMinutes * 60000).toISOString()
    : null;

  await env.DB.prepare(`
    INSERT INTO login_attempts(login_name, attempts, first_attempt_at, blocked_until)
    VALUES (?, ?, ?, ?)
    ON CONFLICT(login_name) DO UPDATE SET
      attempts = excluded.attempts,
      first_attempt_at = excluded.first_attempt_at,
      blocked_until = excluded.blocked_until
  `).bind(loginName, attempts, firstAt, blockedUntil).run();
}

async function clearLoginAttempts(env, loginName) {
  await env.DB.prepare("DELETE FROM login_attempts WHERE login_name = ?").bind(loginName).run();
}

async function logout(request, env) {
  const token = bearerToken(request);
  if (!token) return json({ ok: true });
  await env.DB.prepare("DELETE FROM sessions WHERE token_hash = ?").bind(await sha256(token)).run();
  return json({ ok: true });
}

async function listUsers(identity, env) {
  const result = await env.DB.prepare(`
    SELECT id, name, login, role, active, created_at, updated_at
    FROM users
    WHERE organization_id = ?
    ORDER BY active DESC, CASE role WHEN 'admin' THEN 0 ELSE 1 END, lower(name)
  `).bind(identity.organizationID).all();
  return json({ users: result.results.map((user) => ({ ...user, active: Boolean(user.active) })) });
}

async function organizationStatus(identity, env) {
  const organization = await readOrganization(env, identity.organizationID);
  if (!organization) return json({ error: "not_found", message: "Empresa nao encontrada." }, 404);
  return json({ organization, user: publicUser(identity) });
}

async function updateOrganization(request, identity, env) {
  const body = await readJSON(request);
  const name = text(body.name);
  if (name.length < 2) {
    return json({ error: "invalid_name", message: "Informe o nome da empresa." }, 400);
  }
  const now = new Date().toISOString();
  await env.DB.prepare("UPDATE organizations SET name = ?, updated_at = ? WHERE id = ?")
    .bind(name, now, identity.organizationID)
    .run();
  await writeAudit(env, identity, "organization.updated", "organization", identity.organizationID, { name });
  const organization = await readOrganization(env, identity.organizationID);
  return json({ organization, user: publicUser(identity) });
}

async function createUser(request, identity, env) {
  const body = await readJSON(request);
  const name = text(body.name);
  const loginName = normalizeLogin(body.login);
  const password = text(body.password);
  const requestedRole = text(body.role, "technician");
  const validation = validateUserInput(name, loginName, password, "technician");
  if (validation) return json(validation, 400);
  if (requestedRole !== "technician") {
    return json({ error: "invalid_role", message: "Novos acessos devem ser criados como tecnicos." }, 400);
  }

  const now = new Date().toISOString();
  const user = {
    id: crypto.randomUUID(),
    organizationID: identity.organizationID,
    name,
    login: loginName,
    role: "technician",
    active: true,
  };
  try {
    await env.DB.prepare(`
      INSERT INTO users(id, organization_id, name, login, password_hash, role, active, created_at, updated_at)
      VALUES (?, ?, ?, ?, ?, ?, 1, ?, ?)
    `).bind(user.id, user.organizationID, user.name, user.login, await hashPassword(password), user.role, now, now).run();
  } catch (error) {
    if (String(error).includes("UNIQUE constraint failed")) {
      return json({ error: "login_exists", message: "Esse login ja esta em uso." }, 409);
    }
    throw error;
  }
  await writeAudit(env, identity, "user.created", "user", user.id, { role: user.role, login: user.login });
  return json({ user: publicUser(user) }, 201);
}

async function updateUser(request, identity, userID, env) {
  const body = await readJSON(request);
  const target = await env.DB.prepare(`
    SELECT id, organization_id, name, login, role, active
    FROM users
    WHERE id = ? AND organization_id = ?
    LIMIT 1
  `).bind(userID, identity.organizationID).first();
  if (!target) return json({ error: "not_found", message: "Usuario nao encontrado." }, 404);

  const role = text(body.role, target.role);
  if (!roles.has(role)) return json({ error: "invalid_role", message: "Perfil invalido." }, 400);
  const active = typeof body.active === "boolean" ? body.active : Boolean(target.active);
  if ((!active || role !== "admin") && target.role === "admin" && Boolean(target.active)) {
    const count = await env.DB.prepare(`
      SELECT COUNT(*) AS count FROM users
      WHERE organization_id = ? AND role = 'admin' AND active = 1 AND id <> ?
    `).bind(identity.organizationID, target.id).first();
    if (Number(count?.count || 0) === 0) {
      return json({ error: "last_admin", message: "Mantenha pelo menos um administrador ativo." }, 400);
    }
  }

  // Troca de senha. Ate aqui o worker so lia papel e situacao: o app dizia
  // "Usuario salvo", gravava auditoria, e nem o hash local nem o da nuvem
  // mudavam. Na pratica nao existia troca de senha, e uma senha vazada
  // continuava valendo.
  const password = text(body.password);
  if (password && password.length < 8) {
    return json({ error: "weak_password", message: "A senha precisa de 8 caracteres ou mais." }, 400);
  }

  const now = new Date().toISOString();
  if (password) {
    const passwordHash = await hashPassword(password);
    await env.DB.prepare(`
      UPDATE users SET role = ?, active = ?, password_hash = ?, updated_at = ? WHERE id = ?
    `).bind(role, active ? 1 : 0, passwordHash, now, target.id).run();
    // Uma senha trocada tem que derrubar as sessoes abertas com a senha antiga,
    // senao quem foi desligado continua dentro pelo token que ja tem.
    await env.DB.prepare("DELETE FROM sessions WHERE user_id = ?").bind(target.id).run();
    await clearLoginAttempts(env, normalizeLogin(target.login));
    await writeAudit(env, identity, "user.password_changed", "user", target.id, {});
  } else {
    await env.DB.prepare(`
      UPDATE users SET role = ?, active = ?, updated_at = ? WHERE id = ?
    `).bind(role, active ? 1 : 0, now, target.id).run();
  }

  const action = target.role !== role
    ? (role === "admin" ? "user.promoted_admin" : "user.demoted_technician")
    : (active ? "user.activated" : "user.deactivated");
  await writeAudit(env, identity, action, "user", target.id, { role, active });
  return json({ user: publicUser({ ...toIdentity(target), role, active }) });
}

async function pullChanges(request, identity, env) {
  const url = new URL(request.url);
  const after = Math.max(0, Number.parseInt(url.searchParams.get("after") || "0", 10) || 0);
  const requestedLimit = Number.parseInt(url.searchParams.get("limit") || "100", 10) || 100;
  const limit = Math.min(Math.max(requestedLimit, 1), maxSyncChanges);
  const result = await env.DB.prepare(`
    SELECT sequence, entity_type, entity_id, action, revision, changed_at
    FROM sync_changes
    WHERE organization_id = ? AND sequence > ?
    ORDER BY sequence ASC
    LIMIT ?
  `).bind(identity.organizationID, after, limit).all();

  const changes = [];
  let cursor = after;
  for (const change of result.results) {
    cursor = Number(change.sequence);
    const entity = await readSyncEntity(env, identity.organizationID, change.entity_type, change.entity_id);
    changes.push({
      sequence: cursor,
      entity_type: change.entity_type,
      entity_id: change.entity_id,
      action: change.action,
      revision: Number(change.revision),
      changed_at: change.changed_at,
      entity,
    });
  }
  return json({ cursor, changes, has_more: changes.length === limit });
}

async function pushChanges(request, identity, env) {
  const body = await readJSON(request);
  if (!Array.isArray(body.changes)) {
    return json({ error: "invalid_changes", message: "Envie uma lista de alteracoes." }, 400);
  }
  if (body.changes.length > maxSyncChanges) {
    return json({ error: "too_many_changes", message: `Envie no maximo ${maxSyncChanges} alteracoes por vez.` }, 400);
  }

  // Erro isolado por alteracao. Antes, uma OS sem cliente fazia o worker
  // devolver 400 para o lote todo; o aparelho nao tirava nada da fila e, como o
  // envio vem antes da leitura, parava tambem de receber. Para sempre.
  const results = [];
  for (const change of body.changes) {
    try {
      results.push(await applySyncChange(change, identity, env));
    } catch (err) {
      const message = err instanceof APIError ? err.message : "Falha ao aplicar esta alteracao.";
      results.push({
        entity_id: text(change?.entity_id),
        entity_type: text(change?.entity_type),
        status: "rejected",
        message,
      });
    }
  }
  return json({ results });
}

async function applySyncChange(change, identity, env) {
  const entityType = text(change?.entity_type);
  const entityID = text(change?.entity_id);
  const action = text(change?.action, "upsert");
  const baseRevision = Number(change?.base_revision || 0);
  if (!syncEntityTypes.has(entityType) || !validSyncID(entityID) || !["upsert", "delete"].includes(action) || baseRevision < 0) {
    return { entity_id: entityID, entity_type: entityType, status: "rejected", message: "Alteracao invalida." };
  }

  const current = await readSyncEntity(env, identity.organizationID, entityType, entityID);
  if (current && Number(current.revision) !== baseRevision) {
    return {
      entity_id: entityID,
      entity_type: entityType,
      status: "conflict",
      revision: Number(current.revision),
      entity: current,
    };
  }
  if (!current && action === "delete") {
    return { entity_id: entityID, entity_type: entityType, status: "accepted", revision: 0 };
  }
  if (action === "upsert" && (!change.payload || typeof change.payload !== "object" || Array.isArray(change.payload))) {
    return { entity_id: entityID, entity_type: entityType, status: "rejected", message: "Dados da alteracao invalidos." };
  }

  // So administrador exclui. Essa regra existia apenas nos botoes do app: com
  // o proprio token, que fica em texto claro no SQLite do aparelho, dava para
  // mandar a exclusao de qualquer OS e ela sumia para todo mundo.
  if (action === "delete" && identity.role !== "admin") {
    return {
      entity_id: entityID,
      entity_type: entityType,
      status: "rejected",
      message: "Somente um administrador pode excluir.",
    };
  }

  const now = new Date().toISOString();
  const revision = Number(current?.revision || 0) + 1;
  const write = action === "delete"
    ? softDeleteSyncEntity(env, identity, entityType, entityID, revision, now)
    : upsertSyncEntity(env, identity, entityType, entityID, change.payload, revision, now, !current);

  // As duas gravacoes vao juntas: a entidade e o registro no diario. O registro
  // so entra se a entidade tiver mesmo ficado na revisao nova, que e como um
  // envio simultaneo de outro aparelho vira conflito em vez de sumico.
  await env.DB.batch([
    write,
    recordSyncChange(env, identity, entityType, entityID, action, revision, now),
  ]);

  // O conflito e detectado pela revisao que ficou gravada, e nao pela contagem
  // de linhas do driver: se outro aparelho gravou entre a leitura e a escrita,
  // a guarda de revisao barrou esta gravacao e a entidade continua na revisao
  // antiga. Esta leitura ja acontecia de qualquer jeito.
  const entity = await readSyncEntity(env, identity.organizationID, entityType, entityID);
  if (!entity || Number(entity.revision) !== revision) {
    return {
      entity_id: entityID,
      entity_type: entityType,
      status: "conflict",
      revision: Number(entity?.revision || 0),
      entity,
    };
  }

  await writeAudit(env, identity, `sync.${action}`, entityType, entityID, { revision });
  return { entity_id: entityID, entity_type: entityType, status: "accepted", revision, entity };
}

function validSyncID(value) {
  return /^[A-Za-z0-9_-]{8,80}$/.test(value);
}

async function readSyncEntity(env, organizationID, entityType, entityID) {
  if (entityType === "customer") {
    const row = await env.DB.prepare(`
      SELECT id, name, document, phone, avatar_key, avatar_url, revision, created_at, updated_at, deleted_at
      FROM customers WHERE organization_id = ? AND id = ?
    `).bind(organizationID, entityID).first();
    return row ? publicCustomer(row) : null;
  }
  if (entityType === "service_order") {
    const row = await env.DB.prepare(`
      SELECT id, status, customer_id, customer_name, customer_document, customer_phone, device, brand_model,
             serial, issue, accessories, notes, gross_amount_cents, cost_amount_cents, technician_id,
             checklist_json, photos_json, device_key, delivered_at, paid_at, payment_method, contacted_at,
             contact_name, contact_phone, paid_cents,
             revision, created_at, updated_at, deleted_at
      FROM service_orders WHERE organization_id = ? AND id = ?
    `).bind(organizationID, entityID).first();
    return row ? publicServiceOrder(row) : null;
  }
  if (entityType === "sales_term") {
    const row = await env.DB.prepare(`
      SELECT id, term_type, status, payload_json, amount_cents, pdf_key, signed_key,
             revision, created_at, updated_at, deleted_at
      FROM sales_terms WHERE organization_id = ? AND id = ?
    `).bind(organizationID, entityID).first();
    return row ? publicSalesTerm(row) : null;
  }
  if (entityType === "checklist_template") {
    const row = await env.DB.prepare(`
      SELECT id, name, device_type, items_json, revision, created_at, updated_at, deleted_at
      FROM checklist_templates WHERE organization_id = ? AND id = ?
    `).bind(organizationID, entityID).first();
    return row ? publicChecklistTemplate(row) : null;
  }
  return null;
}

// upsertSyncEntity monta a instrucao de gravacao e a devolve, em vez de
// executa-la. Quem chama junta essa instrucao com o registro no diario de
// alteracoes numa transacao so: antes eram duas chamadas soltas, e o worker
// morrer entre as duas deixava a nuvem com o dado novo e sem registro nenhum,
// de modo que os outros aparelhos nunca recebiam aquela alteracao.
//
// A atualizacao tambem passou a exigir a revisao esperada. Sem isso dois
// aparelhos que enviavam no mesmo segundo passavam os dois pela conferencia e
// os dois gravavam: uma das edicoes sumia, e nenhum dos dois recebia aviso de
// conflito.
function upsertSyncEntity(env, identity, entityType, entityID, payload, revision, now, isNew) {
  if (entityType === "customer") {
    const value = normalizeCustomerPayload(payload);
    if (!value.name) throw new APIError(400, "invalid_customer", "Informe o nome do cliente.");
    if (isNew) {
      return env.DB.prepare(`
        INSERT INTO customers(id, organization_id, name, document, phone, avatar_key, avatar_url, revision, created_by, updated_by, created_at, updated_at)
        VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
      `).bind(entityID, identity.organizationID, value.name, value.document, value.phone, value.avatar_key, value.avatar_url, revision, identity.id, identity.id, now, now);
    }
    return env.DB.prepare(`
      UPDATE customers
      SET name = ?, document = ?, phone = ?, avatar_key = ?, avatar_url = ?, revision = ?, updated_by = ?, updated_at = ?, deleted_at = NULL
      WHERE organization_id = ? AND id = ? AND revision = ?
    `).bind(value.name, value.document, value.phone, value.avatar_key, value.avatar_url, revision, identity.id, now, identity.organizationID, entityID, revision - 1);
  }

  if (entityType === "service_order") {
    const value = normalizeOrderPayload(payload);
    if (!value.customer_name || !value.device || !value.issue) {
      throw new APIError(400, "invalid_order", "Cliente, equipamento e motivo sao obrigatorios.");
    }
    if (isNew) {
      return env.DB.prepare(`
        INSERT INTO service_orders(
          id, organization_id, status, customer_id, customer_name, customer_document, customer_phone,
          device, brand_model, serial, issue, accessories, notes, gross_amount_cents, cost_amount_cents,
          technician_id, checklist_json, photos_json, device_key, delivered_at, paid_at, payment_method,
          contacted_at, contact_name, contact_phone, paid_cents,
          revision, created_by, updated_by, created_at, updated_at
        ) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
      `).bind(
        entityID, identity.organizationID, value.status, value.customer_id, value.customer_name, value.customer_document, value.customer_phone,
        value.device, value.brand_model, value.serial, value.issue, value.accessories, value.notes, value.gross_amount_cents,
        value.cost_amount_cents, value.technician_id, value.checklist_json, value.photos_json,
        value.device_key, value.delivered_at, value.paid_at, value.payment_method, value.contacted_at,
        value.contact_name, value.contact_phone, value.paid_cents,
        revision, identity.id, identity.id, now, now,
      );
    }
    return env.DB.prepare(`
      UPDATE service_orders SET
        status = ?, customer_id = ?, customer_name = ?, customer_document = ?, customer_phone = ?,
        device = ?, brand_model = ?, serial = ?, issue = ?, accessories = ?, notes = ?,
        gross_amount_cents = ?, cost_amount_cents = ?, technician_id = ?, checklist_json = ?, photos_json = ?,
        -- Campo vazio nao apaga o que ja existe. As versoes antigas do app
        -- continuam em campo (o Android esta na 0.7.1) e enviam a OS sem estes
        -- campos: sem esta guarda, uma edicao feita la limparia o fechamento e
        -- a senha do aparelho para todo mundo.
        device_key = CASE WHEN ? <> '' THEN ? ELSE device_key END,
        delivered_at = CASE WHEN ? <> '' THEN ? ELSE delivered_at END,
        paid_at = CASE WHEN ? <> '' THEN ? ELSE paid_at END,
        payment_method = CASE WHEN ? <> '' THEN ? ELSE payment_method END,
        contacted_at = CASE WHEN ? <> '' THEN ? ELSE contacted_at END,
        contact_name = CASE WHEN ? <> '' THEN ? ELSE contact_name END,
        contact_phone = CASE WHEN ? <> '' THEN ? ELSE contact_phone END,
        paid_cents = CASE WHEN ? <> 0 THEN ? ELSE paid_cents END,
        revision = ?, updated_by = ?, updated_at = ?, deleted_at = NULL
      WHERE organization_id = ? AND id = ? AND revision = ?
    `).bind(
      value.status, value.customer_id, value.customer_name, value.customer_document, value.customer_phone,
      value.device, value.brand_model, value.serial, value.issue, value.accessories, value.notes,
      value.gross_amount_cents, value.cost_amount_cents, value.technician_id, value.checklist_json, value.photos_json,
      value.device_key, value.device_key,
      value.delivered_at, value.delivered_at,
      value.paid_at, value.paid_at,
      value.payment_method, value.payment_method,
      value.contacted_at, value.contacted_at,
      value.contact_name, value.contact_name,
      value.contact_phone, value.contact_phone,
      value.paid_cents, value.paid_cents,
      revision, identity.id, now, identity.organizationID, entityID, revision - 1,
    );
  }

  if (entityType === "checklist_template") {
    const value = normalizeChecklistTemplatePayload(payload);
    if (!value.name) throw new APIError(400, "invalid_checklist_template", "Informe o nome da lista.");
    if (isNew) {
      return env.DB.prepare(`
        INSERT INTO checklist_templates(id, organization_id, name, device_type, items_json, revision, created_by, updated_by, created_at, updated_at)
        VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
      `).bind(entityID, identity.organizationID, value.name, value.device_type, value.items_json, revision, identity.id, identity.id, now, now);
    }
    return env.DB.prepare(`
      UPDATE checklist_templates SET name = ?, device_type = ?, items_json = ?, revision = ?, updated_by = ?, updated_at = ?, deleted_at = NULL
      WHERE organization_id = ? AND id = ? AND revision = ?
    `).bind(value.name, value.device_type, value.items_json, revision, identity.id, now, identity.organizationID, entityID, revision - 1);
  }

  const value = normalizeTermPayload(payload);
  if (!value.term_type) throw new APIError(400, "invalid_term", "Informe o tipo do termo.");
  if (isNew) {
    return env.DB.prepare(`
      INSERT INTO sales_terms(id, organization_id, term_type, status, payload_json, amount_cents, pdf_key, signed_key, revision, created_by, updated_by, created_at, updated_at)
      VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
    `).bind(entityID, identity.organizationID, value.term_type, value.status, JSON.stringify(value.payload), value.amount_cents, value.pdf_key, value.signed_key, revision, identity.id, identity.id, now, now);
  }
  return env.DB.prepare(`
    UPDATE sales_terms SET term_type = ?, status = ?, payload_json = ?, amount_cents = ?, pdf_key = ?, signed_key = ?,
      revision = ?, updated_by = ?, updated_at = ?, deleted_at = NULL
    WHERE organization_id = ? AND id = ? AND revision = ?
  `).bind(value.term_type, value.status, JSON.stringify(value.payload), value.amount_cents, value.pdf_key, value.signed_key, revision, identity.id, now, identity.organizationID, entityID, revision - 1);
}

// syncEntityTables liga o tipo sincronizado a sua tabela. Fica fora das funcoes
// porque duas precisam dele, e um nome de tabela entra em SQL por interpolacao:
// e a unica fonte permitida desse valor.
const syncEntityTables = {
  customer: "customers",
  service_order: "service_orders",
  sales_term: "sales_terms",
  checklist_template: "checklist_templates",
};

function softDeleteSyncEntity(env, identity, entityType, entityID, revision, now) {
  const table = syncEntityTables[entityType];
  return env.DB.prepare(`
    UPDATE ${table} SET revision = ?, updated_by = ?, updated_at = ?, deleted_at = ?
    WHERE organization_id = ? AND id = ? AND revision = ?
  `).bind(revision, identity.id, now, now, identity.organizationID, entityID, revision - 1);
}

// recordSyncChange so registra se a entidade estiver mesmo na revisao nova. E
// o que transforma "outro aparelho gravou primeiro" em conflito detectado: a
// gravacao nao pegou, o registro nao entra, e quem chamou ve zero linhas.
function recordSyncChange(env, identity, entityType, entityID, action, revision, now) {
  const table = syncEntityTables[entityType];
  return env.DB.prepare(`
    INSERT INTO sync_changes(organization_id, entity_type, entity_id, action, revision, changed_by, changed_at)
    SELECT ?, ?, ?, ?, ?, ?, ?
    WHERE EXISTS (
      SELECT 1 FROM ${table}
      WHERE organization_id = ? AND id = ? AND revision = ?
    )
  `).bind(
    identity.organizationID, entityType, entityID, action, revision, identity.id, now,
    identity.organizationID, entityID, revision,
  );
}

function normalizeCustomerPayload(payload) {
  return {
    name: text(payload.name),
    document: text(payload.document),
    phone: text(payload.phone),
    avatar_key: text(payload.avatar_key),
    avatar_url: text(payload.avatar_url),
  };
}

function normalizeOrderPayload(payload) {
  return {
    status: text(payload.status, "Aguardando aprovação"),
    customer_id: text(payload.customer_id) || null,
    customer_name: text(payload.customer_name),
    customer_document: text(payload.customer_document),
    customer_phone: text(payload.customer_phone),
    device: text(payload.device),
    brand_model: text(payload.brand_model),
    serial: text(payload.serial),
    issue: text(payload.issue),
    accessories: text(payload.accessories),
    notes: text(payload.notes),
    gross_amount_cents: Math.max(0, Number.parseInt(payload.gross_amount_cents, 10) || 0),
    cost_amount_cents: Math.max(0, Number.parseInt(payload.cost_amount_cents, 10) || 0),
    technician_id: text(payload.technician_id) || null,
    checklist_json: jsonTextOrEmpty(payload.checklist),
    photos_json: jsonTextOrEmpty(payload.photos),
    device_key: text(payload.device_key),
    delivered_at: text(payload.delivered_at),
    paid_at: text(payload.paid_at),
    payment_method: text(payload.payment_method),
    contacted_at: text(payload.contacted_at),
    contact_name: text(payload.contact_name),
    contact_phone: text(payload.contact_phone),
    paid_cents: Math.max(0, Number.parseInt(payload.paid_cents, 10) || 0),
  };
}

// jsonTextOrEmpty serializa um objeto/array para TEXT (D1), ou "" se vazio/nulo.
function jsonTextOrEmpty(value) {
  if (value === null || value === undefined) return "";
  if (typeof value === "string") return value.trim();
  if (Array.isArray(value) && value.length === 0) return "";
  try {
    return JSON.stringify(value);
  } catch {
    return "";
  }
}

function parseJsonOrNull(raw) {
  const value = text(raw);
  if (!value) return null;
  try {
    return JSON.parse(value);
  } catch {
    return null;
  }
}

function normalizeChecklistTemplatePayload(payload) {
  let items = [];
  if (Array.isArray(payload.items)) {
    items = payload.items.map((item) => text(item)).filter((item) => item.length > 0);
  }
  return {
    name: text(payload.name),
    device_type: text(payload.device_type),
    items_json: JSON.stringify(items),
  };
}

function normalizeTermPayload(payload) {
  return {
    term_type: text(payload.term_type),
    status: text(payload.status, "Rascunho"),
    payload: payload.payload && typeof payload.payload === "object" && !Array.isArray(payload.payload) ? payload.payload : {},
    amount_cents: Math.max(0, Number.parseInt(payload.amount_cents, 10) || 0),
    pdf_key: text(payload.pdf_key),
    signed_key: text(payload.signed_key),
  };
}

function publicCustomer(row) {
  return {
    id: row.id,
    name: row.name,
    document: row.document,
    phone: row.phone,
    avatar_key: row.avatar_key,
    avatar_url: row.avatar_url,
    revision: Number(row.revision),
    created_at: row.created_at,
    updated_at: row.updated_at,
    deleted_at: row.deleted_at || null,
  };
}

function publicServiceOrder(row) {
  return {
    id: row.id,
    status: row.status,
    customer_id: row.customer_id || "",
    customer_name: row.customer_name,
    customer_document: row.customer_document,
    customer_phone: row.customer_phone,
    device: row.device,
    brand_model: row.brand_model,
    serial: row.serial,
    issue: row.issue,
    accessories: row.accessories,
    notes: row.notes,
    gross_amount_cents: Number(row.gross_amount_cents),
    cost_amount_cents: Number(row.cost_amount_cents),
    technician_id: row.technician_id || "",
    checklist: parseJsonOrNull(row.checklist_json),
    photos: parseJsonOrNull(row.photos_json) || [],
    device_key: row.device_key || "",
    delivered_at: row.delivered_at || "",
    paid_at: row.paid_at || "",
    payment_method: row.payment_method || "",
    contacted_at: row.contacted_at || "",
    contact_name: row.contact_name || "",
    contact_phone: row.contact_phone || "",
    paid_cents: Number(row.paid_cents || 0),
    revision: Number(row.revision),
    created_at: row.created_at,
    updated_at: row.updated_at,
    deleted_at: row.deleted_at || null,
  };
}

function publicSalesTerm(row) {
  let payload = {};
  try {
    payload = JSON.parse(row.payload_json || "{}");
  } catch {
    payload = {};
  }
  return {
    id: row.id,
    term_type: row.term_type,
    status: row.status,
    payload,
    amount_cents: Number(row.amount_cents),
    pdf_key: row.pdf_key,
    signed_key: row.signed_key,
    revision: Number(row.revision),
    created_at: row.created_at,
    updated_at: row.updated_at,
    deleted_at: row.deleted_at || null,
  };
}

function publicChecklistTemplate(row) {
  let items = [];
  try {
    const parsed = JSON.parse(row.items_json || "[]");
    if (Array.isArray(parsed)) items = parsed;
  } catch {
    items = [];
  }
  return {
    id: row.id,
    name: row.name,
    device_type: row.device_type || "",
    items,
    revision: Number(row.revision),
    created_at: row.created_at,
    updated_at: row.updated_at,
    deleted_at: row.deleted_at || null,
  };
}

async function requireIdentity(request, env) {
  const token = bearerToken(request);
  if (!token) throw new APIError(401, "unauthorized", "Sessao ausente ou expirada.");

  const now = new Date().toISOString();
  const row = await env.DB.prepare(`
    SELECT u.id, u.organization_id, u.name, u.login, u.role, u.active, s.id AS session_id
    FROM sessions s
    INNER JOIN users u ON u.id = s.user_id
    WHERE s.token_hash = ? AND s.expires_at > ? AND u.active = 1
    LIMIT 1
  `).bind(await sha256(token), now).first();
  if (!row) throw new APIError(401, "unauthorized", "Sessao ausente ou expirada.");

  await env.DB.prepare("UPDATE sessions SET last_seen_at = ? WHERE id = ?").bind(now, row.session_id).run();
  return toIdentity(row);
}

async function requireAdmin(request, env) {
  const identity = await requireIdentity(request, env);
  if (identity.role !== "admin") {
    throw new APIError(403, "forbidden", "Acesso restrito ao administrador.");
  }
  return identity;
}

async function createSession(identity, env) {
  const token = randomToken();
  const now = new Date();
  const expiresAt = new Date(now.getTime() + SESSION_DAYS * 24 * 60 * 60 * 1000).toISOString();
  await env.DB.prepare(`
    INSERT INTO sessions(id, user_id, token_hash, expires_at, created_at, last_seen_at)
    VALUES (?, ?, ?, ?, ?, ?)
  `).bind(crypto.randomUUID(), identity.id, await sha256(token), expiresAt, now.toISOString(), now.toISOString()).run();
  return { token, expires_at: expiresAt };
}

async function writeAudit(env, identity, action, entityType, entityID, details = {}) {
  await env.DB.prepare(`
    INSERT INTO audit_events(id, organization_id, actor_user_id, action, entity_type, entity_id, details_json, created_at)
    VALUES (?, ?, ?, ?, ?, ?, ?, ?)
  `).bind(
    crypto.randomUUID(),
    identity.organizationID,
    identity.id,
    action,
    entityType,
    entityID,
    JSON.stringify(details),
    new Date().toISOString(),
  ).run();
}

function publicUser(user) {
  return {
    id: user.id,
    organization_id: user.organizationID,
    name: user.name,
    login: user.login,
    role: user.role,
    active: Boolean(user.active),
  };
}

async function readOrganization(env, organizationID) {
  const row = await env.DB.prepare(`
    SELECT id, name, created_at, updated_at
    FROM organizations
    WHERE id = ?
    LIMIT 1
  `).bind(organizationID).first();
  if (!row) return null;
  return {
    id: row.id,
    name: row.name,
    created_at: row.created_at,
    updated_at: row.updated_at,
  };
}

function toIdentity(row) {
  return {
    id: row.id,
    organizationID: row.organization_id,
    name: row.name,
    login: row.login,
    role: row.role,
    active: Boolean(row.active),
  };
}

function validateUserInput(name, loginName, password, role) {
  if (!name) return { error: "invalid_name", message: "Informe o nome do usuario." };
  if (!/^[a-z0-9._@-]{3,64}$/.test(loginName)) {
    return { error: "invalid_login", message: "Login invalido." };
  }
  if (password.length < 8) {
    return { error: "invalid_password", message: "A senha precisa ter pelo menos 8 caracteres." };
  }
  if (!roles.has(role)) return { error: "invalid_role", message: "Perfil invalido." };
  return null;
}

async function hashPassword(password) {
  const salt = crypto.getRandomValues(new Uint8Array(16));
  const hash = await derivePassword(password, salt, PBKDF2_ITERATIONS);
  return `pbkdf2_sha256$${PBKDF2_ITERATIONS}$${base64url(salt)}$${base64url(hash)}`;
}

async function verifyPassword(password, encoded) {
  const [algorithm, iterations, saltEncoded, hashEncoded] = String(encoded).split("$");
  if (algorithm !== "pbkdf2_sha256" || !iterations || !saltEncoded || !hashEncoded) return false;
  const expected = base64urlBytes(hashEncoded);
  const actual = await derivePassword(password, base64urlBytes(saltEncoded), Number(iterations));
  return constantTimeBytesEqual(actual, expected);
}

async function derivePassword(password, salt, iterations) {
  const key = await crypto.subtle.importKey("raw", new TextEncoder().encode(password), "PBKDF2", false, ["deriveBits"]);
  const bits = await crypto.subtle.deriveBits(
    { name: "PBKDF2", hash: "SHA-256", salt, iterations },
    key,
    256,
  );
  return new Uint8Array(bits);
}

async function sha256(value) {
  const digest = await crypto.subtle.digest("SHA-256", new TextEncoder().encode(value));
  return [...new Uint8Array(digest)].map((byte) => byte.toString(16).padStart(2, "0")).join("");
}

function bearerToken(request) {
  const value = request.headers.get("Authorization") || "";
  return value.startsWith("Bearer ") ? value.slice(7).trim() : "";
}

async function readJSON(request) {
  try {
    const value = await request.json();
    return value && typeof value === "object" ? value : {};
  } catch {
    throw new APIError(400, "invalid_json", "JSON invalido.");
  }
}

function text(value, fallback = "") {
  return typeof value === "string" && value.trim() ? value.trim() : fallback;
}

function normalizeLogin(value) {
  return text(value).toLowerCase();
}

function randomToken() {
  return base64url(crypto.getRandomValues(new Uint8Array(32)));
}

function base64url(bytes) {
  let value = "";
  for (const byte of bytes) value += String.fromCharCode(byte);
  return btoa(value).replace(/\+/g, "-").replace(/\//g, "_").replace(/=+$/g, "");
}

function base64urlBytes(value) {
  const padded = value.replace(/-/g, "+").replace(/_/g, "/") + "=".repeat((4 - (value.length % 4)) % 4);
  const binary = atob(padded);
  return Uint8Array.from(binary, (char) => char.charCodeAt(0));
}

function constantTimeBytesEqual(a, b) {
  if (a.length !== b.length) return false;
  let difference = 0;
  for (let index = 0; index < a.length; index += 1) difference |= a[index] ^ b[index];
  return difference === 0;
}

function constantTimeEqual(a, b) {
  return constantTimeBytesEqual(new TextEncoder().encode(a), new TextEncoder().encode(b));
}

function json(payload, status = 200) {
  return new Response(JSON.stringify(payload), {
    status,
    headers: { "content-type": "application/json; charset=utf-8", "cache-control": "no-store" },
  });
}

class APIError extends Error {
  constructor(status, error, message) {
    super(message);
    this.status = status;
    this.error = error;
  }
}
