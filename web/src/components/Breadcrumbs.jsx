// items: [{ label, onClick? }] - the last item (no onClick) renders as the
// current page; every earlier one is a clickable jump back to that level.
export default function Breadcrumbs({ items }) {
  return (
    <nav className="breadcrumbs">
      {items.map((item, i) => (
        <span key={i} className="breadcrumb-segment">
          {i > 0 && <span className="breadcrumb-sep">/</span>}
          {item.onClick ? (
            <button type="button" className="breadcrumb-link" onClick={item.onClick}>{item.label}</button>
          ) : (
            <span className="breadcrumb-current">{item.label}</span>
          )}
        </span>
      ))}
    </nav>
  );
}
