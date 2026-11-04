DROP TABLE IF EXISTS idempotency_keys, lot_settings, admin_actions, admin_users, email_notifications,
  outbox_events, gate_scans, tickets, gate_devices, pricing_rules, parking_spots, vehicles, email_codes, owners CASCADE;
DROP SEQUENCE IF EXISTS ticket_number_seq;
DROP TYPE IF EXISTS scan_result, gate_kind, ticket_status, spot_status, spot_type, vehicle_status, vehicle_type;
