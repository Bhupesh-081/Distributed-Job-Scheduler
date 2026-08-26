// A designed "nothing here yet" card instead of a lone line of muted
// text - used on first-run screens (orgs/projects) where a fresh account
// would otherwise be one gray sentence in a lot of empty space.
export default function EmptyState({ icon: Icon, title, description, action }) {
  return (
    <div className="card empty-state">
      {Icon && <div className="empty-state-icon"><Icon size={26} /></div>}
      <h3>{title}</h3>
      {description && <p className="muted">{description}</p>}
      {action}
    </div>
  );
}
