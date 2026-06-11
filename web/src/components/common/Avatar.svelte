<script>
  /**
   * Circular avatar with a consistent generated colour from a text hash.
   *
   * Props:
   *   name  {string}  Text to hash for colour and initials (handle, group name, etc.)
   *   size  {number}  Diameter in pixels (default 40)
   */
  export let name = '';
  export let size = 40;

  /** Generate a deterministic HSL colour from a string. */
  function hashColour(str) {
    let hash = 0;
    for (let i = 0; i < str.length; i++) {
      hash = str.charCodeAt(i) + ((hash << 5) - hash);
    }
    const h = ((hash % 360) + 360) % 360;
    return `hsl(${h}, 55%, 45%)`;
  }

  /** Extract the first 2 characters of the name, uppercased. */
  function initials(str) {
    const clean = str.trim();
    if (!clean) return '?';
    const parts = clean.split(/\s+/);
    if (parts.length >= 2) {
      return (parts[0][0] + parts[1][0]).toUpperCase();
    }
    return clean.slice(0, 2).toUpperCase();
  }

  $: bg = hashColour(name);
  $: chars = initials(name);
  $: px = `${size}px`;
  $: fontSize = `${Math.round(size * 0.4)}px`;
</script>

<div
  class="avatar"
  role="img"
  aria-label={name || 'Avatar'}
  style="width: {px}; height: {px}; background-color: {bg}; font-size: {fontSize};"
>
  {chars}
</div>

<style>
  .avatar {
    display: flex;
    align-items: center;
    justify-content: center;
    border-radius: 50%;
    color: #fff;
    font-weight: 700;
    line-height: 1;
    user-select: none;
    flex-shrink: 0;
    overflow: hidden;
  }
</style>
